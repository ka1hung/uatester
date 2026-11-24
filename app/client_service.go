package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ka1hung/opcua/client"
	"github.com/ka1hung/opcua/ua"
	"github.com/ka1hung/uatester/core"
)

// ClientService is the client side of the window.
type ClientService struct {
	logs   *logHub
	emit   func(name string, data any)
	pkiDir string

	mu     sync.Mutex
	c      *client.Client
	stop   chan struct{}
	sub    *client.Subscription
	stress context.CancelFunc
}

// ClientParams are the connection settings in the window.
type ClientParams struct {
	Endpoint  string `json:"endpoint"`
	Security  string `json:"security"`
	User      string `json:"user"`
	Password  string `json:"password"`
	TimeoutMs int    `json:"timeoutMs"`
}

func (s *ClientService) config(p ClientParams) core.ClientConfig {
	if p.TimeoutMs <= 0 {
		p.TimeoutMs = 5000
	}
	return core.ClientConfig{Endpoint: strings.TrimSpace(p.Endpoint), Security: p.Security, User: strings.TrimSpace(p.User),
		Password: p.Password, Timeout: time.Duration(p.TimeoutMs) * time.Millisecond, PKIDir: s.pkiDir}
}

// Securities lists the security choices.
func (s *ClientService) Securities() []string { return core.Securities }

// Connection describes the connected endpoint.
type Connection struct {
	Endpoint string `json:"endpoint"`
	Policy   string `json:"policy"`
	Mode     string `json:"mode"`
}

// Connect connects to a server, closing any earlier connection.
func (s *ClientService) Connect(p ClientParams) (Connection, error) {
	s.Disconnect()
	cfg := s.config(p)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout+5*time.Second)
	defer cancel()
	s.logs.printf("client", "info", "connecting to %s (%s)", cfg.Endpoint, p.Security)
	c, err := core.Dial(ctx, cfg)
	if err != nil {
		s.logs.printf("client", "error", "connect: %v", err)
		return Connection{}, err
	}
	ep := c.Endpoint()
	conn := Connection{Endpoint: ep.EndpointURL,
		Policy: strings.TrimPrefix(ep.SecurityPolicyURI, "http://opcfoundation.org/UA/SecurityPolicy#"),
		Mode:   strings.TrimPrefix(ep.SecurityMode.String(), "MessageSecurityMode")}
	stop := make(chan struct{})
	s.mu.Lock()
	s.c, s.stop = c, stop
	s.mu.Unlock()
	go s.watchState(c, stop)
	if conn.Policy == "None" {
		s.logs.printf("client", "info", "connected: no security")
	} else {
		s.logs.printf("client", "info", "connected: %s %s", conn.Policy, conn.Mode)
	}
	return conn, nil
}

func (s *ClientService) watchState(c *client.Client, stop chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case st, ok := <-c.State():
			if !ok {
				return
			}
			s.logs.printf("client", "info", "connection state: %v", st)
			s.emit("client:state", st.String())
		}
	}
}

// Disconnect closes the connection.
func (s *ClientService) Disconnect() {
	s.mu.Lock()
	c, stop, sub := s.c, s.stop, s.sub
	s.c, s.stop, s.sub = nil, nil, nil
	s.mu.Unlock()
	if c == nil {
		return
	}
	close(stop)
	if sub != nil {
		_ = sub.Close()
	}
	_ = c.Close()
	s.logs.printf("client", "info", "disconnected")
	s.emit("client:state", "Closed")
}

func (s *ClientService) conn() (*client.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.c == nil {
		return nil, errors.New("not connected")
	}
	return s.c, nil
}

func (s *ClientService) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// Node is one node in the browse tree.
type Node struct {
	NodeID      string `json:"nodeId"`
	BrowseName  string `json:"browseName"`
	DisplayName string `json:"displayName"`
	NodeClass   string `json:"nodeClass"`
}

// Browse lists a node's children.
func (s *ClientService) Browse(nodeID string) ([]Node, error) {
	c, err := s.conn()
	if err != nil {
		return nil, err
	}
	ctx, cancel := s.ctx()
	defer cancel()
	items, err := c.Browse(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	out := make([]Node, 0, len(items))
	for _, it := range items {
		out = append(out, Node{NodeID: it.NodeID, BrowseName: it.BrowseName, DisplayName: it.DisplayName,
			NodeClass: strings.TrimPrefix(it.NodeClass.String(), "NodeClass")})
	}
	return out, nil
}

// Value is one node's value.
type Value struct {
	NodeID string    `json:"nodeId"`
	Value  any       `json:"value"`
	Type   string    `json:"type"`
	Status string    `json:"status"`
	Good   bool      `json:"good"`
	Time   time.Time `json:"time"`
	Error  string    `json:"error,omitempty"`
}

func toValue(r client.Result) Value {
	v := Value{NodeID: r.NodeID, Value: display(r.Value), Status: core.StatusName(r.Status), Good: r.Good(), Time: r.SourceTime}
	if r.Value != nil {
		v.Type = strings.TrimPrefix(fmt.Sprintf("%T", r.Value), "*")
	}
	if r.Err != nil {
		v.Error = r.Err.Error()
	}
	return v
}

// display turns values the window cannot show as they are into text.
func display(v any) any {
	switch x := v.(type) {
	case nil, bool, string, time.Time,
		int8, int16, int32, int64, uint8, uint16, uint32, uint64, float32, float64:
		return v
	case *ua.LocalizedText:
		if x == nil {
			return nil
		}
		return x.Text
	case *ua.QualifiedName:
		if x == nil {
			return nil
		}
		return x.Name
	case fmt.Stringer:
		return x.String()
	}
	return fmt.Sprint(v)
}

// Read reads values.
func (s *ClientService) Read(nodeIDs []string) ([]Value, error) {
	c, err := s.conn()
	if err != nil {
		return nil, err
	}
	ctx, cancel := s.ctx()
	defer cancel()
	var out []Value
	for _, r := range c.ReadMany(ctx, nodeIDs) {
		out = append(out, toValue(r))
	}
	return out, nil
}

// Write writes a value given as text; the type is taken from the node.
func (s *ClientService) Write(nodeID, value string) error {
	c, err := s.conn()
	if err != nil {
		return err
	}
	ctx, cancel := s.ctx()
	defer cancel()
	if err := c.WriteAuto(ctx, nodeID, value); err != nil {
		s.logs.printf("client", "error", "write %s := %s: %v", nodeID, value, err)
		return err
	}
	s.logs.printf("client", "info", "write %s := %s ok", nodeID, value)
	return nil
}

// Watch subscribes to the nodes, replacing the earlier subscription. Each
// change arrives as a "client:value" event. An empty list unsubscribes.
func (s *ClientService) Watch(nodeIDs []string, intervalMs int) ([]Value, error) {
	c, err := s.conn()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	old := s.sub
	s.sub = nil
	s.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	if len(nodeIDs) == 0 {
		return nil, nil
	}
	ctx, cancel := s.ctx()
	defer cancel()
	sub, results, err := c.Subscribe(ctx, nodeIDs, time.Duration(intervalMs)*time.Millisecond, func(id string, r client.Result) {
		r.NodeID = id
		s.emit("client:value", toValue(r))
	})
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.sub = sub
	s.mu.Unlock()
	var out []Value
	for _, r := range results {
		out = append(out, toValue(r))
	}
	return out, nil
}

// History reads a node's history over the last fromMinutes: raw values,
// or an aggregate with one value per intervalSeconds (0: one value).
func (s *ClientService) History(nodeID, aggregate string, fromMinutes, intervalSeconds int) ([]core.HistoryValue, error) {
	c, err := s.conn()
	if err != nil {
		return nil, err
	}
	ctx, cancel := s.ctx()
	defer cancel()
	vals, err := core.History(ctx, c, strings.TrimSpace(nodeID), aggregate,
		time.Duration(fromMinutes)*time.Minute, time.Duration(intervalSeconds)*time.Second)
	for i := range vals {
		vals[i].Value = display(vals[i].Value)
	}
	return vals, err
}

// StressParams configure a load test from the window.
type StressParams struct {
	Clients     int `json:"clients"`
	Points      int `json:"points"`
	IntervalMs  int `json:"intervalMs"`
	DurationSec int `json:"durationSec"`
}

// StartStress starts a load test against the endpoint in p. Progress
// arrives as "stress" events.
func (s *ClientService) StartStress(p ClientParams, st StressParams) error {
	s.mu.Lock()
	if s.stress != nil {
		s.mu.Unlock()
		return errors.New("a load test is already running")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.stress = cancel
	s.mu.Unlock()

	cfg := s.config(p)
	sc := core.StressConfig{Clients: st.Clients, Points: st.Points,
		Interval: time.Duration(st.IntervalMs) * time.Millisecond, Duration: time.Duration(st.DurationSec) * time.Second}
	s.logs.printf("client", "info", "load test: %d clients x %d points every %d ms for %d s", st.Clients, st.Points, st.IntervalMs, st.DurationSec)
	go func() {
		err := core.Stress(ctx, cfg, sc, 2*time.Second, func(r core.StressReport) { s.emit("stress", r) })
		if err != nil {
			s.logs.printf("client", "error", "load test: %v", err)
			s.emit("stress", core.StressReport{Done: true, LastError: err.Error()})
		} else {
			s.logs.printf("client", "info", "load test finished")
		}
		s.mu.Lock()
		s.stress = nil
		s.mu.Unlock()
		cancel()
	}()
	return nil
}

// StopStress ends a running load test early.
func (s *ClientService) StopStress() {
	s.mu.Lock()
	cancel := s.stress
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// ServiceShutdown closes the connection when the window closes.
func (s *ClientService) ServiceShutdown() error {
	s.StopStress()
	s.Disconnect()
	return nil
}
