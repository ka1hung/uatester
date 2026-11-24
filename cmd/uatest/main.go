// Command uatest is the command line version of uatester: a simulated OPC
// UA server and a client for diagnostics and load tests. It is pure Go and
// cross-compiles to Linux armv7, for soak tests on the field controller.
//
//	uatest server    [-addr 0.0.0.0:4840 -host 192.168.1.10 -points 3000 -interval 500ms -history 200 -stats 1m]
//	uatest endpoints -ep opc.tcp://host:4840
//	uatest status    -ep ...
//	uatest browse    -ep ... [-node i=85]
//	uatest attrs     -ep ... -node ns=2;s=P0000_Temp
//	uatest read      -ep ... -node ns=2;s=P0000_Temp [-node ...]
//	uatest write     -ep ... -node ns=2;s=Setpoint -value 23 -user admin -pass admin
//	uatest subscribe -ep ... -node ns=2;s=P0000_Temp [-interval 500ms -deadband 0.5 | -pctdeadband 1]
//	uatest call      -ep ... -object ns=2;s=Plant -method ns=2;s=Plant.Reset -arg "reason" -user admin -pass admin
//	uatest events    -ep ... [-node ns=2;s=Plant] [-ack -comment "seen"]
//	uatest history   -ep ... -node ns=2;s=P0000_Temp [-agg average -from 1h -interval 10m]
//	uatest history   -ep ... -node ns=2;s=Plant -agg events -from 1h
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

var usage = `uatest ` + core.Version + ` - OPC UA test tool

usage:
  uatest server    run a simulated server
  uatest endpoints list a server's endpoints (security and logins)
  uatest status    show a server's state and product
  uatest browse    list the children of a node
  uatest attrs     show a node's attributes
  uatest read      read values
  uatest write     write a value
  uatest subscribe print value changes
  uatest call      call a method
  uatest events    print events and alarms; -ack acknowledges alarms
  uatest history   read history: raw, an aggregate, or events
  uatest stress    load test: many clients subscribing
  uatest version   print the version

uatest <command> -h shows the command's flags.
`

type list []string

func (n *list) String() string     { return strings.Join(*n, ",") }
func (n *list) Set(s string) error { *n = append(*n, s); return nil }

func main() {
	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "help" {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if os.Args[1] == "version" || os.Args[1] == "-version" {
		fmt.Println("uatest", core.Version)
		return
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
	var nodes, callArgs list
	fs.Var(&nodes, "node", "node id (repeatable); events: the notifier (default: the Server object)")
	value := fs.String("value", "", "write: value to write")
	interval := fs.Duration("interval", 500*time.Millisecond, "subscribe, events, stress: publishing interval; history: aggregate interval")
	deadband := fs.Float64("deadband", 0, "subscribe: ignore changes smaller than this")
	pctDeadband := fs.Float64("pctdeadband", 0, "subscribe: ignore changes smaller than this percent of the EURange")
	object := fs.String("object", "", "call: object id")
	method := fs.String("method", "", "call: method id")
	fs.Var(&callArgs, "arg", "call: argument (repeatable, converted to the method's types)")
	ack := fs.Bool("ack", false, "events: acknowledge every active alarm not yet acknowledged")
	comment := fs.String("comment", "acknowledged by uatest", "events: comment for -ack")
	agg := fs.String("agg", "raw", "history: raw, average, minimum, maximum, count, start, end, or events")
	from := fs.Duration("from", time.Hour, "history: how far back to read")
	var st core.StressConfig
	fs.IntVar(&st.Clients, "clients", 10, "stress: clients")
	fs.IntVar(&st.Points, "points", 3000, "stress: points per client (ns=2;s=P0000_Temp ...)")
	fs.DurationVar(&st.Duration, "duration", time.Minute, "stress: how long")
	_ = fs.Parse(args)

	switch cmd {
	case "endpoints":
		eps, err := client.Endpoints(ctx, c.Endpoint, c.Timeout)
		if err != nil {
			return err
		}
		for _, e := range eps {
			fmt.Printf("%-24s %-15s level %-3d logins %s\n", e.Policy, e.Mode, e.SecurityLevel, strings.Join(e.UserTokens, ","))
		}
		return nil
	case "stress":
		st.Interval = *interval
		return core.Stress(ctx, c, st, 10*time.Second, func(r core.StressReport) {
			fmt.Printf("%6.0fs  %8d notifications/s  connected %d  failed %d  tool goroutines %d heap %d MB\n",
				r.Elapsed.Seconds(), r.PerSecond, r.Connected, r.Failed, r.Goroutines, r.HeapMB)
			if r.LastError != "" && r.Done {
				fmt.Println("last error:", r.LastError)
			}
		})
	case "status", "browse", "attrs", "read", "write", "subscribe", "call", "events", "history":
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
	fmt.Fprintf(os.Stderr, "connected: %s %s\n", strings.TrimPrefix(ep.SecurityPolicyURI, "http://opcfoundation.org/UA/SecurityPolicy#"),
		strings.TrimPrefix(ep.SecurityMode.String(), "MessageSecurityMode"))

	switch cmd {
	case "status":
		s, err := cl.ServerStatus(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("state       %s\nproduct     %s %s (%s)\nmanufacturer %s\nstarted     %s\nserver time %s\n",
			s.State, s.ProductName, s.SoftwareVersion, s.ProductURI, s.ManufacturerName,
			s.StartTime.Local().Format(time.DateTime), s.CurrentTime.Local().Format(time.DateTime))
		ns, err := cl.Namespaces(ctx)
		if err != nil {
			return err
		}
		for i, n := range ns {
			fmt.Printf("ns=%d        %s\n", i, n)
		}
	case "browse":
		if len(nodes) == 0 {
			nodes = list{"i=85"}
		}
		for _, n := range nodes {
			items, err := cl.Browse(ctx, n)
			if err != nil {
				return err
			}
			for _, it := range items {
				fmt.Printf("%-40s %-30s %v\n", it.NodeID, it.BrowseName, strings.TrimPrefix(it.NodeClass.String(), "NodeClass"))
			}
		}
	case "attrs":
		for _, n := range nodes {
			a, err := cl.Attributes(ctx, n)
			if err != nil {
				return err
			}
			printAttrs(a)
			if a.NodeClass.String() == "NodeClassMethod" {
				in, out, err := cl.MethodArguments(ctx, n)
				if err != nil {
					return err
				}
				for _, x := range in {
					fmt.Printf("  in   %-16s %s\n", x.Name, x.DataType)
				}
				for _, x := range out {
					fmt.Printf("  out  %-16s %s\n", x.Name, x.DataType)
				}
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
		o := client.SubscribeOptions{Interval: *interval, AbsoluteDeadband: *deadband, PercentDeadband: *pctDeadband}
		sub, results, err := cl.SubscribeWith(ctx, nodes, o, func(id string, r client.Result) {
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
		return waitStates(ctx, cl)
	case "call":
		if *object == "" || *method == "" {
			return fmt.Errorf("-object and -method are required")
		}
		args := make([]any, len(callArgs))
		for i, a := range callArgs {
			args[i] = a
		}
		out, err := cl.Call(ctx, *object, *method, args...)
		if err != nil {
			return err
		}
		fmt.Printf("ok, %d output arguments\n", len(out))
		for i, v := range out {
			fmt.Printf("  out %d = %v\n", i+1, v)
		}
	case "events":
		notifier := client.ServerObject
		if len(nodes) > 0 {
			notifier = nodes[0]
		}
		sub, err := cl.SubscribeEvents(ctx, notifier, *interval, func(e client.Event) {
			ev := core.FromClient(e)
			printEvent(ev)
			if *ack && ev.IsCondition() && ev.Active && !ev.Acked {
				go func() {
					if err := cl.Acknowledge(ctx, e, *comment); err != nil {
						fmt.Printf("  acknowledge %s: %v\n", ev.ConditionID, err)
					} else {
						fmt.Printf("  acknowledged %s\n", ev.ConditionID)
					}
				}()
			}
		})
		if err != nil {
			return err
		}
		defer sub.Close()
		if err := sub.Refresh(ctx); err != nil {
			fmt.Println("refresh:", err)
		}
		return waitStates(ctx, cl)
	case "history":
		if len(nodes) == 0 {
			return fmt.Errorf("-node is required")
		}
		iv := *interval
		if !flagSet(fs, "interval") {
			iv = 0
		}
		for _, n := range nodes {
			if *agg == "events" {
				evs, err := cl.HistoryEvents(ctx, n, time.Now().Add(-*from), time.Now(), 0)
				for _, e := range evs {
					printEvent(core.FromClient(e))
				}
				if err != nil {
					return err
				}
				continue
			}
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

func waitStates(ctx context.Context, cl *client.Client) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case s := <-cl.State():
			fmt.Println("state:", s)
		}
	}
}

func printAttrs(a client.NodeInfo) {
	fmt.Printf("node        %s\nclass       %s\nbrowse name %s\ndisplay     %s\n", a.NodeID,
		strings.TrimPrefix(a.NodeClass.String(), "NodeClass"), a.BrowseName, a.DisplayName)
	if a.Description != "" {
		fmt.Printf("description %s\n", a.Description)
	}
	if a.DataType != "" {
		fmt.Printf("data type   %s (value rank %d)\naccess      read %v, write %v, history %v\n",
			a.DataType, a.ValueRank, a.CanRead(), a.CanWrite(), a.HasHistory())
	}
	if a.HasEvents() {
		fmt.Printf("events      yes, history %v\n", a.HasHistory())
	}
	if a.Executable {
		fmt.Printf("executable  %v (this user: %v)\n", a.Executable, a.UserExecutable)
	}
}

func printEvent(e core.Event) {
	switch e.Type {
	case "RefreshStart", "RefreshEnd":
		fmt.Printf("-- %s --\n", e.Type)
		return
	}
	state := ""
	if e.IsCondition() {
		state = map[bool]string{true: "ACTIVE", false: "cleared"}[e.Active]
		state += map[bool]string{true: ", acked", false: ", not acked"}[e.Acked]
		state = " [" + e.ConditionName + " " + state + "]"
	}
	fmt.Printf("%s  sev %-4d %-14s %s%s\n", e.Time.Local().Format("15:04:05.000"), e.Severity, e.SourceName, e.Message, state)
}

func flagSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}
