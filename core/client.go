package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ka1hung/opcua/certs"
	"github.com/ka1hung/opcua/client"
)

// ClientConfig says how to connect to a server.
type ClientConfig struct {
	Endpoint string        `json:"endpoint"` // e.g. opc.tcp://192.168.1.10:4840
	Security string        `json:"security"` // auto (Sign or better, never None), none, or policy:mode
	User     string        `json:"user"`     // anonymous if empty
	Password string        `json:"password"`
	Timeout  time.Duration `json:"timeout"`
	// PKIDir keeps this tool's certificate, so a server has to trust it
	// only once. Empty: a new certificate for every connection.
	PKIDir string `json:"pkiDir"`
}

// Securities lists the choices ClientConfig.Security accepts, for menus.
var Securities = []string{
	"none", "auto",
	"Basic256Sha256:Sign", "Basic256Sha256:SignAndEncrypt",
	"Aes128_Sha256_RsaOaep:Sign", "Aes128_Sha256_RsaOaep:SignAndEncrypt",
	"Aes256_Sha256_RsaPss:Sign", "Aes256_Sha256_RsaPss:SignAndEncrypt",
	"Basic128Rsa15:Sign", "Basic128Rsa15:SignAndEncrypt",
	"Basic256:Sign", "Basic256:SignAndEncrypt",
}

// ClientURI is the ApplicationURI this tool uses as a client.
const ClientURI = "urn:uatester:client"

// Options turns the configuration into client options.
func (c ClientConfig) Options() (client.Options, error) {
	o := client.Options{Security: security(c.Security), Timeout: c.Timeout, ApplicationURI: ClientURI}
	if c.User != "" {
		o.Auth = client.UserPass(c.User, c.Password)
	}
	if c.PKIDir != "" {
		host, _ := os.Hostname()
		cert, key, _, _, err := certs.LoadOrCreate(filepath.Join(c.PKIDir, "cert.der"), filepath.Join(c.PKIDir, "key.pem"),
			certs.Spec{ApplicationURI: ClientURI, Hosts: []string{host}}, false)
		if err != nil {
			return o, err
		}
		o.Certificate, o.Key = cert, key
	}
	return o, nil
}

// Dial connects to the server.
func Dial(ctx context.Context, c ClientConfig) (*client.Client, error) {
	o, err := c.Options()
	if err != nil {
		return nil, err
	}
	return client.Dial(ctx, c.Endpoint, o)
}

func security(s string) client.Security {
	switch strings.ToLower(s) {
	case "", "none":
		return client.NoSecurity()
	case "auto":
		return client.Auto(client.MinSign)
	}
	p, m, _ := strings.Cut(s, ":")
	return client.Explicit(p, m)
}

// HistoryValue is one value read from history.
type HistoryValue struct {
	Time   time.Time `json:"time"`
	Value  any       `json:"value"`
	Status string    `json:"status"`
}

// History reads a node's history over the last from: raw values (agg
// "raw" or empty), or with an aggregate (average, minimum, maximum,
// count, start, end) one value per interval (0: one for the whole range).
func History(ctx context.Context, c *client.Client, node, agg string, from, interval time.Duration) ([]HistoryValue, error) {
	end := time.Now()
	start := end.Add(-from)
	var vals []client.HistoryValue
	var err error
	if agg == "" || agg == "raw" {
		vals, err = c.HistoryRaw(ctx, node, start, end, 0)
	} else {
		a, perr := client.ParseAggregate(agg)
		if perr != nil {
			return nil, perr
		}
		vals, err = c.HistoryProcessed(ctx, node, a, start, end, interval)
	}
	out := make([]HistoryValue, 0, len(vals))
	for _, v := range vals {
		out = append(out, HistoryValue{Time: v.SourceTime, Value: v.Value, Status: StatusName(v.Status & 0xFFFF0000)})
	}
	return out, err
}

// StressConfig is a load test: Clients clients each subscribe to the
// first Points points of this tool's server (ns=2;s=P0000_Temp ...).
type StressConfig struct {
	Clients  int           `json:"clients"`
	Points   int           `json:"points"`
	Interval time.Duration `json:"interval"`
	Duration time.Duration `json:"duration"`
}

// StressReport is the load test's progress.
type StressReport struct {
	Elapsed    time.Duration `json:"elapsed"`
	PerSecond  int64         `json:"perSecond"` // notifications per second since the last report
	Total      int64         `json:"total"`
	Connected  int           `json:"connected"`
	Failed     int           `json:"failed"`
	Goroutines int           `json:"goroutines"` // of this tool
	HeapMB     uint64        `json:"heapMB"`     // of this tool
	Done       bool          `json:"done"`
	LastError  string        `json:"lastError,omitempty"`
}

// Stress runs a load test and calls report every interval of reportEvery
// and once more when it ends.
func Stress(ctx context.Context, c ClientConfig, s StressConfig, reportEvery time.Duration, report func(StressReport)) error {
	o, err := c.Options()
	if err != nil {
		return err
	}
	if s.Clients <= 0 || s.Points <= 0 {
		return fmt.Errorf("clients and points must be positive")
	}
	ids := make([]string, s.Points)
	for i := range ids {
		ids[i] = fmt.Sprintf("ns=2;s=P%04d_Temp", i)
	}
	ctx, cancel := context.WithTimeout(ctx, s.Duration)
	defer cancel()

	var notes atomic.Int64
	var connected, failed atomic.Int32
	var lastErr atomic.Value
	var wg sync.WaitGroup
	for k := 0; k < s.Clients; k++ {
		wg.Add(1)
		go func(k int) {
			defer wg.Done()
			fail := func(err error) {
				if ctx.Err() == nil {
					failed.Add(1)
					lastErr.Store(fmt.Sprintf("client %d: %v", k, err))
				}
			}
			cl, err := client.Dial(ctx, c.Endpoint, o)
			if err != nil {
				fail(err)
				return
			}
			defer cl.Close()
			sub, _, err := cl.Subscribe(ctx, ids, s.Interval, func(string, client.Result) { notes.Add(1) })
			if err != nil {
				fail(err)
				return
			}
			defer sub.Close()
			connected.Add(1)
			<-ctx.Done()
		}(k)
	}

	start := time.Now()
	t := time.NewTicker(reportEvery)
	defer t.Stop()
	var last int64
	snapshot := func(done bool) StressReport {
		n := notes.Load()
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		r := StressReport{Elapsed: time.Since(start).Round(time.Second), Total: n,
			PerSecond: int64(float64(n-last) / reportEvery.Seconds()),
			Connected: int(connected.Load()), Failed: int(failed.Load()),
			Goroutines: runtime.NumGoroutine(), HeapMB: ms.HeapAlloc >> 20, Done: done}
		if e, ok := lastErr.Load().(string); ok {
			r.LastError = e
		}
		last = n
		return r
	}
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			r := snapshot(true)
			r.PerSecond = int64(float64(r.Total) / time.Since(start).Seconds())
			report(r)
			return nil
		case <-t.C:
			report(snapshot(false))
		}
	}
}
