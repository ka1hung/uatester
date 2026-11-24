// Command uatest is the command line version of uatester: a simulated OPC
// UA server and a client for diagnostics and load tests. It is pure Go and
// cross-compiles to Linux armv7, for soak tests on the field controller.
//
//	uatest server    [-addr 0.0.0.0:4840 -host 192.168.1.10 -points 3000 -interval 500ms -history 200 -stats 1m]
//	uatest browse    -ep opc.tcp://host:4840 [-node i=85]
//	uatest read      -ep ... -node ns=2;s=P0000_Temp [-node ...]
//	uatest write     -ep ... -node ns=2;s=Setpoint -value 23 -user admin -pass admin
//	uatest subscribe -ep ... -node ns=2;s=P0000_Temp [-interval 500ms]
//	uatest history   -ep ... -node ns=2;s=P0000_Temp [-agg average -from 1h -interval 10m]
//	uatest stress    -ep ... -clients 10 -points 3000 -interval 500ms -duration 5m
//
// Client security: -security none (default), auto (Sign or better, never
// None), or policy:mode such as Basic256Sha256:SignAndEncrypt.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/ka1hung/opcua/client"
	"github.com/ka1hung/uatester/core"
)

const usage = `uatest ` + core.Version + ` - OPC UA test tool

usage:
  uatest server    run a simulated server
  uatest browse    list the children of a node
  uatest read      read values
  uatest write     write a value
  uatest subscribe print value changes
  uatest history   read history (raw or aggregate)
  uatest stress    load test: many clients subscribing

uatest <command> -h shows the command's flags.
`

type nodeList []string

func (n *nodeList) String() string     { return strings.Join(*n, ",") }
func (n *nodeList) Set(s string) error { *n = append(*n, s); return nil }

func main() {
	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "help" {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var err error
	if cmd := os.Args[1]; cmd == "server" {
		err = runServer(ctx, os.Args[2:])
	} else {
		err = runClient(ctx, cmd, os.Args[2:])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runServer(ctx context.Context, args []string) error {
	cfg := core.DefaultServerConfig()
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "listen address")
	fs.StringVar(&cfg.Host, "host", "", "host name or IP clients use (default: this host's name)")
	fs.StringVar(&cfg.PKIDir, "pki", cfg.PKIDir, "certificate directory")
	fs.IntVar(&cfg.Points, "points", cfg.Points, "number of simulated points")
	fs.DurationVar(&cfg.Interval, "interval", cfg.Interval, "update interval")
	fs.BoolVar(&cfg.AutoTrust, "autotrust", false, "trust every client certificate (commissioning only)")
	fs.BoolVar(&cfg.Legacy, "legacy", false, "also offer Basic128Rsa15 and Basic256 (deprecated)")
	fs.IntVar(&cfg.History, "history", cfg.History, "values kept per point for history (0: no history)")
	statsEvery := fs.Duration("stats", time.Minute, "how often to log load statistics")
	verbose := fs.Bool("v", false, "debug logging")
	_ = fs.Parse(args)

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	s, err := core.StartServer(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer s.Close()
	logger.Info("users: admin/admin (read, write, acknowledge), viewer/viewer and anonymous (read only)")

	t := time.NewTicker(*statsEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			l := s.Load()
			logger.Info("stats", "connections", l.Connections, "sessions", len(l.Sessions), "subscriptions", l.Subscriptions,
				"goroutines", l.Goroutines, "heapMB", l.HeapMB, "sysMB", l.SysMB)
		}
	}
}

func runClient(ctx context.Context, cmd string, args []string) error {
	var c core.ClientConfig
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	fs.StringVar(&c.Endpoint, "ep", "opc.tcp://localhost:4840", "endpoint URL")
	fs.StringVar(&c.Security, "security", "none", "none, auto, or policy:mode")
	fs.StringVar(&c.User, "user", "", "user name (anonymous if empty)")
	fs.StringVar(&c.Password, "pass", "", "password")
	fs.DurationVar(&c.Timeout, "timeout", 5*time.Second, "request timeout")
	fs.StringVar(&c.PKIDir, "pki", "", "keep this tool's certificate in this directory, so a server trusts it once")
	var nodes nodeList
	fs.Var(&nodes, "node", "node id (repeatable)")
	value := fs.String("value", "", "write: value to write")
	interval := fs.Duration("interval", 500*time.Millisecond, "subscribe, stress: publishing interval; history: aggregate interval")
	agg := fs.String("agg", "raw", "history: raw, average, minimum, maximum, count, start or end")
	from := fs.Duration("from", time.Hour, "history: how far back to read")
	var st core.StressConfig
	fs.IntVar(&st.Clients, "clients", 10, "stress: clients")
	fs.IntVar(&st.Points, "points", 3000, "stress: points per client (ns=2;s=P0000_Temp ...)")
	fs.DurationVar(&st.Duration, "duration", time.Minute, "stress: how long")
	_ = fs.Parse(args)

	switch cmd {
	case "stress":
		st.Interval = *interval
		return core.Stress(ctx, c, st, 10*time.Second, func(r core.StressReport) {
			fmt.Printf("%6.0fs  %8d notifications/s  connected %d  failed %d  tool goroutines %d heap %d MB\n",
				r.Elapsed.Seconds(), r.PerSecond, r.Connected, r.Failed, r.Goroutines, r.HeapMB)
			if r.LastError != "" && r.Done {
				fmt.Println("last error:", r.LastError)
			}
		})
	case "browse", "read", "write", "subscribe", "history":
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	cl, err := core.Dial(ctx, c)
	if err != nil {
		return err
	}
	defer cl.Close()
	ep := cl.Endpoint()
	fmt.Fprintf(os.Stderr, "connected: %s %s\n", strings.TrimPrefix(ep.SecurityPolicyURI, "http://opcfoundation.org/UA/SecurityPolicy#"), ep.SecurityMode)

	switch cmd {
	case "browse":
		if len(nodes) == 0 {
			nodes = nodeList{"i=85"}
		}
		for _, n := range nodes {
			items, err := cl.Browse(ctx, n)
			if err != nil {
				return err
			}
			for _, it := range items {
				fmt.Printf("%-40s %-30s %v\n", it.NodeID, it.BrowseName, it.NodeClass)
			}
		}
	case "read":
		for _, r := range cl.ReadMany(ctx, nodes) {
			if r.Err != nil {
				fmt.Printf("%s: error: %v\n", r.NodeID, r.Err)
				continue
			}
			fmt.Printf("%s = %v  [%s]  %s\n", r.NodeID, r.Value, core.StatusName(r.Status), r.SourceTime.Local().Format(time.RFC3339Nano))
		}
	case "write":
		for _, n := range nodes {
			if err := cl.WriteAuto(ctx, n, *value); err != nil {
				return err
			}
			fmt.Printf("%s := %s ok\n", n, *value)
		}
	case "subscribe":
		sub, results, err := cl.Subscribe(ctx, nodes, *interval, func(id string, r client.Result) {
			fmt.Printf("%s %s = %v [%s]\n", time.Now().Format("15:04:05.000"), id, r.Value, core.StatusName(r.Status))
		})
		if err != nil {
			return err
		}
		defer sub.Close()
		for _, r := range results {
			if r.Err != nil || !r.Good() {
				fmt.Printf("%s: %s %v\n", r.NodeID, core.StatusName(r.Status), r.Err)
			}
		}
		for {
			select {
			case <-ctx.Done():
				return nil
			case s := <-cl.State():
				fmt.Println("state:", s)
			}
		}
	case "history":
		if len(nodes) == 0 {
			return fmt.Errorf("-node is required")
		}
		iv := *interval
		if !flagSet(fs, "interval") {
			iv = 0
		}
		for _, n := range nodes {
			vals, err := core.History(ctx, cl, n, *agg, *from, iv)
			for _, v := range vals {
				fmt.Printf("%s  %s  %v  [%s]\n", n, v.Time.Local().Format("2006-01-02 15:04:05.000"), v.Value, v.Status)
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func flagSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}
