package core

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"
)

// startTestServer starts a simulated server on a free local port.
func startTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	cfg := DefaultServerConfig()
	cfg.Addr, cfg.Host = fmt.Sprintf("127.0.0.1:%d", port), "127.0.0.1"
	cfg.PKIDir, cfg.Points, cfg.Interval = t.TempDir(), 20, 50*time.Millisecond
	s, err := StartServer(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, fmt.Sprintf("opc.tcp://127.0.0.1:%d", port)
}

func TestServerAndClient(t *testing.T) {
	s, ep := startTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if n := len(s.Endpoints()); n != 7 {
		t.Errorf("endpoints: got %d, want 7 (None + 3 policies x 2 modes)", n)
	}

	viewer, err := Dial(ctx, ClientConfig{Endpoint: ep, User: ViewerUser, Password: ViewerUser})
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	if err := viewer.WriteAuto(ctx, "ns=2;s=Setpoint", 30); err == nil {
		t.Error("viewer could write")
	}

	admin, err := Dial(ctx, ClientConfig{Endpoint: ep, Security: "Basic256Sha256:SignAndEncrypt",
		User: AdminUser, Password: AdminUser, PKIDir: t.TempDir()})
	if err == nil {
		admin.Close()
		t.Fatal("server accepted an untrusted client certificate")
	}

	admin, err = Dial(ctx, ClientConfig{Endpoint: ep, User: AdminUser, Password: AdminUser})
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if err := admin.WriteAuto(ctx, "ns=2;s=Setpoint", 30); err != nil {
		t.Fatalf("admin write: %v", err)
	}
	if err := admin.WriteAuto(ctx, "ns=2;s=P0003_Temp", 12.5); err != nil {
		t.Fatalf("admin write: %v", err)
	}

	// A simulated point a client wrote keeps the written value.
	time.Sleep(200 * time.Millisecond)
	r := admin.ReadMany(ctx, []string{"ns=2;s=Setpoint", "ns=2;s=P0003_Temp"})
	if r[0].Value != int16(30) || r[1].Value != 12.5 {
		t.Errorf("after write: got %v, %v", r[0].Value, r[1].Value)
	}

	// Set by hand from the tool, then back to the simulation.
	if err := s.SetPoint("P0005_Temp", "99.5"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPoint("Running", "false"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPoint("Setpoint", "abc"); err == nil {
		t.Error("Setpoint accepted abc")
	}
	time.Sleep(200 * time.Millisecond)
	for _, p := range s.Points() {
		switch p.Name {
		case "P0005_Temp":
			if p.Value != 99.5 || !p.Manual {
				t.Errorf("P0005_Temp: %+v", p)
			}
		case "Running":
			if p.Value != false {
				t.Errorf("Running: %+v", p)
			}
		}
	}
	if err := s.ResumePoint("P0005_Temp"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if r := admin.ReadMany(ctx, []string{"ns=2;s=P0005_Temp"}); r[0].Value == 99.5 {
		t.Error("P0005_Temp did not return to the simulation")
	}

	// History: raw values and an aggregate.
	raw, err := History(ctx, admin, "ns=2;s=P0001_Temp", "raw", time.Minute, 0)
	if err != nil || len(raw) == 0 {
		t.Errorf("raw history: %d values, %v", len(raw), err)
	}
	avg, err := History(ctx, admin, "ns=2;s=P0001_Temp", "average", time.Minute, 0)
	if err != nil || len(avg) != 1 {
		t.Errorf("average: %v, %v", avg, err)
	}

	// Alarms raised by hand show up and can be acknowledged.
	if err := s.SetAlarm("P0002_Temp", true, 0, ""); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range s.Alarms() {
		if a.Source == "P0002_Temp" && a.Name == ManualAlarm && a.Active {
			found = true
		}
	}
	if !found {
		t.Errorf("manual alarm not listed: %+v", s.Alarms())
	}
	if err := s.Acknowledge("P0002_Temp", ManualAlarm, "test"); err != nil {
		t.Errorf("acknowledge: %v", err)
	}

	if l := s.Load(); len(l.Sessions) != 2 {
		t.Errorf("sessions: got %d, want 2", len(l.Sessions))
	}
}

func TestStress(t *testing.T) {
	_, ep := startTestServer(t)
	var last StressReport
	err := Stress(context.Background(), ClientConfig{Endpoint: ep},
		StressConfig{Clients: 3, Points: 20, Interval: 100 * time.Millisecond, Duration: 2 * time.Second},
		500*time.Millisecond, func(r StressReport) { last = r })
	if err != nil {
		t.Fatal(err)
	}
	if !last.Done || last.Connected != 3 || last.Failed != 0 || last.Total == 0 {
		t.Errorf("final report: %+v", last)
	}
}

func TestTrustRejectedCertificate(t *testing.T) {
	s, ep := startTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg := ClientConfig{Endpoint: ep, Security: "Basic256Sha256:SignAndEncrypt", PKIDir: t.TempDir()}
	if c, err := Dial(ctx, cfg); err == nil {
		c.Close()
		t.Fatal("untrusted certificate accepted")
	}
	rej, err := s.Rejected()
	if err != nil || len(rej) != 1 || rej[0].URI != ClientURI {
		t.Fatalf("rejected: %+v, %v", rej, err)
	}
	if err := s.Trust(rej[0].Thumbprint); err != nil {
		t.Fatal(err)
	}
	c, err := Dial(ctx, cfg)
	if err != nil {
		t.Fatalf("after trusting: %v", err)
	}
	c.Close()
}
