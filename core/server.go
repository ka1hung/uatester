// Package core holds what the command line tool and the desktop app share:
// a simulated OPC UA server and client helpers.
package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ka1hung/opcua/certs"
	"github.com/ka1hung/opcua/server"
	"github.com/ka1hung/opcua/ua"
)

// ServerConfig configures the simulated server.
type ServerConfig struct {
	Addr      string        `json:"addr"`      // listen address, e.g. 0.0.0.0:4840
	Host      string        `json:"host"`      // host name or IP clients use; this host's name if empty
	PKIDir    string        `json:"pkiDir"`    // certificate directory
	Points    int           `json:"points"`    // number of simulated temperature points
	Interval  time.Duration `json:"interval"`  // simulation update interval
	AutoTrust bool          `json:"autoTrust"` // trust every client certificate
	Legacy    bool          `json:"legacy"`    // also offer Basic128Rsa15 and Basic256
	History   int           `json:"history"`   // values kept per point for history; 0 disables history
}

// DefaultServerConfig is a small server on port 4840.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{Addr: "0.0.0.0:4840", PKIDir: "pki", Points: 100, Interval: 500 * time.Millisecond, History: 200}
}

// Users accepted by the simulated server: admin may write and
// acknowledge; viewer and anonymous may only read.
const (
	AdminUser  = "admin"
	ViewerUser = "viewer"
)

// Server is a running simulated OPC UA server. Its temperature points
// follow slow sine waves; every tenth one raises a High alarm above 80.
type Server struct {
	cfg   ServerConfig
	srv   *server.Server
	store *certs.Store
	log   *slog.Logger
	ns    uint16
	area  *ua.NodeID
	hist  *history

	points []*point
	byName map[string]*point

	mu       sync.Mutex
	simulate bool
	alarmWas map[string]string // last state and message of each alarm, to record history only on change

	cancel context.CancelFunc
	done   chan struct{}
}

type point struct {
	name   string
	id     *ua.NodeID
	kind   ua.TypeID
	sim    bool // follows the simulation
	manual bool // set by hand; the simulation leaves it alone
}

// StartServer starts a simulated server. Close stops it.
func StartServer(ctx context.Context, cfg ServerConfig, logger *slog.Logger) (*Server, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 500 * time.Millisecond
	}
	if cfg.Host == "" {
		cfg.Host, _ = os.Hostname()
	}
	_, portStr, err := net.SplitHostPort(cfg.Addr)
	if err != nil {
		return nil, fmt.Errorf("listen address %q: %w", cfg.Addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("listen address %q: bad port", cfg.Addr)
	}

	appURI := "urn:" + cfg.Host + ":uatester:server"
	certFile, keyFile := filepath.Join(cfg.PKIDir, "own", "cert.der"), filepath.Join(cfg.PKIDir, "own", "key.pem")
	cert, key, created, warn, err := certs.LoadOrCreate(certFile, keyFile,
		certs.Spec{ApplicationURI: appURI, Hosts: []string{cfg.Host, "localhost"}}, true)
	if err != nil {
		return nil, err
	}
	if created {
		logger.Info("created server certificate", "file", certFile)
	}
	if warn != nil {
		logger.Warn("server certificate replaced", "reason", warn)
	}
	store, err := certs.Open(cfg.PKIDir, certs.Options{AutoTrust: cfg.AutoTrust})
	if err != nil {
		return nil, err
	}

	opts := []server.Option{
		server.Certificate(cert), server.PrivateKey(key), server.ApplicationURI(appURI),
		server.ServerName("uatester"), server.ProductName("uatester"), server.ManufacturerName("uatester"), server.SoftwareVersion(Version),
		server.EndPoint(cfg.Host, port), server.ListenAddress(cfg.Addr),
		server.EnableSecurity("None", ua.MessageSecurityModeNone),
		server.EnableAuthMode(ua.UserTokenTypeAnonymous),
		server.EnableAuthMode(ua.UserTokenTypeUserName),
		server.UserNameAuthenticator(func(ctx context.Context, user, pass string) (*server.Identity, error) {
			if (user == AdminUser || user == ViewerUser) && pass == user {
				return &server.Identity{Roles: []string{user}}, nil
			}
			return nil, errors.New("bad user or password")
		}),
		server.TrustStore(store),
		server.SetLogger(logger),
	}
	policies := []string{"Basic256Sha256", "Aes128_Sha256_RsaOaep", "Aes256_Sha256_RsaPss"}
	if cfg.Legacy {
		policies = append(policies, "Basic128Rsa15", "Basic256")
	}
	for _, p := range policies {
		opts = append(opts,
			server.EnableSecurity(p, ua.MessageSecurityModeSign),
			server.EnableSecurity(p, ua.MessageSecurityModeSignAndEncrypt))
	}

	s := &Server{cfg: cfg, srv: server.New(opts...), store: store, log: logger, byName: map[string]*point{}, simulate: true, done: make(chan struct{})}
	if err := s.build(); err != nil {
		return nil, err
	}

	ctx, s.cancel = context.WithCancel(ctx)
	if err := s.srv.Start(ctx); err != nil {
		s.cancel()
		return nil, err
	}
	for _, ep := range s.Endpoints() {
		logger.Info("endpoint", "url", ep.URL, "policy", ep.Policy, "mode", ep.Mode)
	}
	go s.run(ctx)
	return s, nil
}

func (s *Server) build() error {
	srv := s.srv
	s.ns = srv.AddNamespace("urn:uatester:points")
	isAdmin := func(who *server.Identity) bool { return len(who.Roles) > 0 && who.Roles[0] == AdminUser }
	area, err := srv.AddObject(server.ObjectsFolder, server.ObjectSpec{
		NodeID: ua.NewStringNodeID(s.ns, "Plant"), BrowseName: "Plant", EventNotifier: true, EventHistory: true,
	})
	if err != nil {
		return err
	}
	s.area = area.ID()
	s.hist = newHistory(s.cfg.History)
	if s.cfg.History > 0 {
		srv.SetHistory(s.hist)
	}

	add := func(p *point, spec server.VariableSpec) error {
		spec.NodeID, spec.BrowseName, spec.Writable, spec.CanWrite = p.id, p.name, true, isAdmin
		spec.DataType = p.kind
		if _, err := srv.AddVariable(s.area, spec); err != nil {
			return err
		}
		s.points = append(s.points, p)
		s.byName[p.name] = p
		return nil
	}
	for i := 0; i < s.cfg.Points; i++ {
		name := fmt.Sprintf("P%04d_Temp", i)
		p := &point{name: name, id: ua.NewStringNodeID(s.ns, name), kind: ua.TypeIDDouble, sim: true}
		if err := add(p, server.VariableSpec{Description: "simulated temperature", Unit: "°C",
			EURange: &server.Range{Low: 0, High: 100}, Historizing: true, Value: 20.0}); err != nil {
			return err
		}
	}
	if err := add(&point{name: "Setpoint", id: ua.NewStringNodeID(s.ns, "Setpoint"), kind: ua.TypeIDInt16},
		server.VariableSpec{Value: int16(22)}); err != nil {
		return err
	}
	if err := add(&point{name: "Running", id: ua.NewStringNodeID(s.ns, "Running"), kind: ua.TypeIDBoolean},
		server.VariableSpec{Value: true}); err != nil {
		return err
	}

	if _, err := srv.AddMethod(s.area, server.MethodSpec{
		NodeID: ua.NewStringNodeID(s.ns, "Plant.Reset"), BrowseName: "Reset",
		InputArguments:  []*ua.Argument{{Name: "Reason", DataType: ua.NewNumericNodeID(0, 12), ValueRank: -1}},
		OutputArguments: []*ua.Argument{{Name: "Done", DataType: ua.NewNumericNodeID(0, 1), ValueRank: -1}},
		CanCall:         isAdmin,
		Handler: func(ctx context.Context, who *server.Identity, obj *ua.NodeID, args []*ua.Variant) ([]*ua.Variant, ua.StatusCode) {
			reason, _ := args[0].Value().(string)
			s.log.Info("method Reset", "user", who.UserName, "reason", reason)
			_ = srv.FireEvent(&server.Event{Source: s.area, Message: "reset by " + who.UserName + ": " + reason, Severity: 300})
			return []*ua.Variant{ua.MustVariant(true)}, ua.StatusOK
		},
	}); err != nil {
		return err
	}
	srv.OnWrite(func(ctx context.Context, who *server.Identity, nid *ua.NodeID, v *ua.Variant) ua.StatusCode {
		s.log.Info("client write", "user", who.UserName, "node", nid.String(), "value", v.Value())
		if p := s.byName[nid.StringID()]; p != nil && p.sim {
			s.mu.Lock()
			p.manual = true // keep what the client wrote
			s.mu.Unlock()
		}
		return ua.StatusOK
	})
	srv.OnAcknowledge(func(ctx context.Context, who *server.Identity, src *ua.NodeID, name, comment string) ua.StatusCode {
		s.log.Info("client acknowledge", "user", who.UserName, "source", src.String(), "alarm", name, "comment", comment)
		return ua.StatusOK
	})
	return nil
}

func (s *Server) run(ctx context.Context) {
	defer close(s.done)
	t := time.NewTicker(s.cfg.Interval)
	defer t.Stop()
	start := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			s.mu.Lock()
			on := s.simulate
			s.mu.Unlock()
			if on {
				s.step(now, now.Sub(start).Seconds())
			}
		}
	}
}

func (s *Server) step(now time.Time, el float64) {
	for i, p := range s.points {
		if !p.sim {
			continue
		}
		s.mu.Lock()
		manual := p.manual
		s.mu.Unlock()
		if manual {
			continue
		}
		v := 50 + 40*math.Sin(el/60+float64(i)) + rand.Float64()
		v = math.Round(v*10) / 10
		s.setValue(p, v, now)
		if i%10 == 0 {
			active := v > 80
			// A fixed message: the server sends an alarm event only when the
			// state changes, not on every new value.
			msg := p.name + " high (> 80)"
			if !active {
				msg = p.name + " back to normal"
			}
			s.setAlarm(p, "High", active, 700, msg, now)
		}
	}
}

func (s *Server) setValue(p *point, v any, now time.Time) {
	_ = s.srv.SetValue(p.id, v, ua.StatusOK, now)
	if s.cfg.History > 0 {
		s.hist.add(p.id, &ua.DataValue{Value: ua.MustVariant(v), SourceTimestamp: now})
	}
}

func (s *Server) setAlarm(p *point, name string, active bool, sev uint16, msg string, now time.Time) error {
	err := s.srv.SetAlarm(server.Alarm{Source: p.id, Name: name, Active: active, Severity: sev, Message: msg, Time: now})
	if err != nil || s.cfg.History == 0 {
		return err
	}
	// Record history when the alarm starts, changes or ends, not on every
	// update that leaves it as it was.
	k, now2 := p.name+"/"+name, fmt.Sprintf("%v|%d|%s", active, sev, msg)
	s.mu.Lock()
	if s.alarmWas == nil {
		s.alarmWas = map[string]string{}
	}
	was, seen := s.alarmWas[k]
	s.alarmWas[k] = now2
	s.mu.Unlock()
	if was != now2 && (active || seen) {
		s.hist.addEvent(&server.Event{Source: p.id, Time: now, Message: msg, Severity: sev})
	}
	return nil
}

// Close stops the server.
func (s *Server) Close() error {
	s.cancel()
	<-s.done
	return s.srv.Close()
}

// Config returns the configuration the server runs with.
func (s *Server) Config() ServerConfig { return s.cfg }

// Endpoint is one endpoint the server offers.
type Endpoint struct {
	URL    string `json:"url"`
	Policy string `json:"policy"`
	Mode   string `json:"mode"`
}

// Endpoints lists the server's endpoints.
func (s *Server) Endpoints() []Endpoint {
	var out []Endpoint
	for _, ep := range s.srv.Endpoints() {
		out = append(out, Endpoint{
			URL:    ep.EndpointURL,
			Policy: strings.TrimPrefix(ep.SecurityPolicyURI, "http://opcfoundation.org/UA/SecurityPolicy#"),
			Mode:   strings.TrimPrefix(ep.SecurityMode.String(), "MessageSecurityMode"),
		})
	}
	return out
}

// Point is a point's current state.
type Point struct {
	Name   string    `json:"name"`
	NodeID string    `json:"nodeId"`
	Type   string    `json:"type"`
	Value  any       `json:"value"`
	Status string    `json:"status"`
	Time   time.Time `json:"time"`
	Manual bool      `json:"manual"` // set by hand or by a client; the simulation leaves it alone
}

// Points lists the points and their values.
func (s *Server) Points() []Point {
	out := make([]Point, 0, len(s.points))
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.points {
		pt := Point{Name: p.name, NodeID: p.id.String(), Type: typeName(p.kind), Manual: p.manual}
		if dv, err := s.srv.Value(p.id); err == nil {
			if dv.Value != nil {
				pt.Value = dv.Value.Value()
			}
			pt.Status, pt.Time = StatusName(dv.Status), dv.SourceTimestamp
		}
		out = append(out, pt)
	}
	return out
}

// SetPoint sets a point by hand from text: a number, or true / false.
// A simulated point then keeps that value until ResumePoint.
func (s *Server) SetPoint(name, text string) error {
	p := s.byName[name]
	if p == nil {
		return fmt.Errorf("no point %q", name)
	}
	text = strings.TrimSpace(text)
	var v any
	var err error
	switch p.kind {
	case ua.TypeIDDouble:
		v, err = strconv.ParseFloat(text, 64)
	case ua.TypeIDInt16:
		var n int64
		n, err = strconv.ParseInt(text, 10, 16)
		v = int16(n)
	case ua.TypeIDBoolean:
		v, err = strconv.ParseBool(text)
	}
	if err != nil {
		return fmt.Errorf("%s: %q is not a %s", name, text, typeName(p.kind))
	}
	s.mu.Lock()
	p.manual = p.sim
	s.mu.Unlock()
	s.setValue(p, v, time.Now())
	s.log.Info("set by hand", "point", name, "value", v)
	return nil
}

// ResumePoint returns a point set by hand to the simulation.
func (s *Server) ResumePoint(name string) error {
	p := s.byName[name]
	if p == nil {
		return fmt.Errorf("no point %q", name)
	}
	s.mu.Lock()
	p.manual = false
	s.mu.Unlock()
	return nil
}

// SetSimulating pauses or resumes the simulation of all points.
func (s *Server) SetSimulating(on bool) {
	s.mu.Lock()
	s.simulate = on
	s.mu.Unlock()
	s.log.Info("simulation", "on", on)
}

// Simulating reports whether the simulation runs.
func (s *Server) Simulating() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.simulate
}

// ManualAlarm is the name of the alarm SetAlarm raises by hand.
const ManualAlarm = "Manual"

// SetAlarm raises or clears the Manual alarm of a point.
func (s *Server) SetAlarm(name string, active bool, severity uint16, message string) error {
	p := s.byName[name]
	if p == nil {
		return fmt.Errorf("no point %q", name)
	}
	if message == "" {
		message = name + " manual alarm"
		if !active {
			message = name + " manual alarm cleared"
		}
	}
	if severity == 0 {
		severity = 500
	}
	s.log.Info("alarm by hand", "point", name, "active", active)
	return s.setAlarm(p, ManualAlarm, active, severity, message, time.Now())
}

// FireEvent sends a plain event from the Plant object.
func (s *Server) FireEvent(message string, severity uint16) error {
	if severity == 0 {
		severity = 300
	}
	return s.srv.FireEvent(&server.Event{Source: s.area, Message: message, Severity: severity})
}

// Alarm is an alarm's current state.
type Alarm struct {
	Source   string    `json:"source"`
	Name     string    `json:"name"`
	Active   bool      `json:"active"`
	Acked    bool      `json:"acked"`
	Severity uint16    `json:"severity"`
	Message  string    `json:"message"`
	Time     time.Time `json:"time"`
	AckedBy  string    `json:"ackedBy"`
}

// Alarms lists the alarms the server keeps: active ones and those not yet
// acknowledged.
func (s *Server) Alarms() []Alarm {
	var out []Alarm
	for _, a := range s.srv.Alarms() {
		out = append(out, Alarm{Source: a.Source.StringID(), Name: a.Name, Active: a.Active, Acked: a.Acked,
			Severity: a.Severity, Message: a.Message, Time: a.Time, AckedBy: a.AckedBy})
	}
	return out
}

// Acknowledge acknowledges an alarm as the operator of this tool.
func (s *Server) Acknowledge(source, name, comment string) error {
	p := s.byName[source]
	if p == nil {
		return fmt.Errorf("no point %q", source)
	}
	return s.srv.AcknowledgeAlarm(p.id, name, comment, "uatester")
}

// Session is one client session.
type Session struct {
	Name          string    `json:"name"`
	Client        string    `json:"client"`
	User          string    `json:"user"`
	Subscriptions int       `json:"subscriptions"`
	LastActivity  time.Time `json:"lastActivity"`
}

// Load is the server's current load.
type Load struct {
	Connections   int       `json:"connections"`
	Subscriptions int       `json:"subscriptions"`
	Sessions      []Session `json:"sessions"`
	Goroutines    int       `json:"goroutines"`
	HeapMB        uint64    `json:"heapMB"`
	SysMB         uint64    `json:"sysMB"`
}

// Load reports connections, sessions and memory.
func (s *Server) Load() Load {
	st := s.srv.Stats()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	l := Load{Connections: st.Connections, Subscriptions: st.Subscriptions,
		Goroutines: runtime.NumGoroutine(), HeapMB: ms.HeapAlloc >> 20, SysMB: ms.Sys >> 20}
	for _, se := range st.Sessions {
		user := se.User
		if user == "" {
			user = "anonymous"
		}
		l.Sessions = append(l.Sessions, Session{Name: se.Name, Client: se.ClientURI, User: user,
			Subscriptions: se.Subscriptions, LastActivity: se.LastActivity})
	}
	return l
}

func typeName(t ua.TypeID) string {
	switch t {
	case ua.TypeIDDouble:
		return "Double"
	case ua.TypeIDInt16:
		return "Int16"
	case ua.TypeIDBoolean:
		return "Boolean"
	}
	return t.String()
}

// StatusName is a status code's short name, such as Good or
// BadUserAccessDenied.
func StatusName(s ua.StatusCode) string {
	if s == ua.StatusOK {
		return "Good"
	}
	if d, ok := ua.StatusCodes[s]; ok {
		return strings.TrimPrefix(d.Name, "Status")
	}
	if d, ok := ua.StatusCodes[s&0xFFFF0000]; ok {
		return strings.TrimPrefix(d.Name, "Status")
	}
	return fmt.Sprintf("0x%08X", uint32(s))
}
