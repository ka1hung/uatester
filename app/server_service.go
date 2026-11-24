package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/ka1hung/uatester/core"
)

// ServerService runs the simulated server for the window.
type ServerService struct {
	logs   *logHub
	pkiDir string

	mu  sync.Mutex
	srv *core.Server
}

// ServerParams are the server settings in the window.
type ServerParams struct {
	Addr       string `json:"addr"`
	Host       string `json:"host"`
	Points     int    `json:"points"`
	IntervalMs int    `json:"intervalMs"`
	History    int    `json:"history"`
	AutoTrust  bool   `json:"autoTrust"`
	Legacy     bool   `json:"legacy"`
}

// ServerStatus is what the window shows about the server.
type ServerStatus struct {
	Running    bool            `json:"running"`
	Simulating bool            `json:"simulating"`
	Endpoints  []core.Endpoint `json:"endpoints"`
	Load       core.Load       `json:"load"`
	PKIDir     string          `json:"pkiDir"`
}

// Defaults returns the initial server settings.
func (s *ServerService) Defaults() ServerParams {
	d := core.DefaultServerConfig()
	return ServerParams{Addr: d.Addr, Points: d.Points, IntervalMs: int(d.Interval / time.Millisecond), History: d.History}
}

// Start starts the server.
func (s *ServerService) Start(p ServerParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv != nil {
		return errors.New("server is already running")
	}
	cfg := core.ServerConfig{Addr: strings.TrimSpace(p.Addr), Host: strings.TrimSpace(p.Host), PKIDir: s.pkiDir,
		Points: p.Points, Interval: time.Duration(p.IntervalMs) * time.Millisecond, History: p.History,
		AutoTrust: p.AutoTrust, Legacy: p.Legacy}
	srv, err := core.StartServer(context.Background(), cfg, s.logs.logger("server"))
	if err != nil {
		s.logs.printf("server", "error", "start: %v", err)
		return err
	}
	s.srv = srv
	return nil
}

// Stop stops the server.
func (s *ServerService) Stop() error {
	s.mu.Lock()
	srv := s.srv
	s.srv = nil
	s.mu.Unlock()
	if srv == nil {
		return nil
	}
	err := srv.Close()
	s.logs.printf("server", "info", "stopped")
	return err
}

func (s *ServerService) running() (*core.Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv == nil {
		return nil, errors.New("server is not running")
	}
	return s.srv, nil
}

// Status reports whether the server runs, its endpoints and its load.
func (s *ServerService) Status() ServerStatus {
	st := ServerStatus{PKIDir: s.pkiDir}
	srv, err := s.running()
	if err != nil {
		return st
	}
	st.Running, st.Simulating = true, srv.Simulating()
	st.Endpoints, st.Load = srv.Endpoints(), srv.Load()
	return st
}

// PointPage is a filtered part of the point list.
type PointPage struct {
	Points []core.Point `json:"points"`
	Total  int          `json:"total"` // points matching the filter
}

// Points lists up to limit points whose name contains filter.
func (s *ServerService) Points(filter string, limit int) (PointPage, error) {
	srv, err := s.running()
	if err != nil {
		return PointPage{}, err
	}
	filter = strings.ToLower(strings.TrimSpace(filter))
	var page PointPage
	for _, p := range srv.Points() {
		if filter != "" && !strings.Contains(strings.ToLower(p.Name), filter) {
			continue
		}
		page.Total++
		if len(page.Points) < limit {
			page.Points = append(page.Points, p)
		}
	}
	return page, nil
}

// SetPoint sets a point by hand.
func (s *ServerService) SetPoint(name, value string) error {
	srv, err := s.running()
	if err != nil {
		return err
	}
	return srv.SetPoint(name, value)
}

// ResumePoint returns a point to the simulation.
func (s *ServerService) ResumePoint(name string) error {
	srv, err := s.running()
	if err != nil {
		return err
	}
	return srv.ResumePoint(name)
}

// SetSimulating pauses or resumes the simulation.
func (s *ServerService) SetSimulating(on bool) error {
	srv, err := s.running()
	if err != nil {
		return err
	}
	srv.SetSimulating(on)
	return nil
}

// Alarms lists the server's alarms.
func (s *ServerService) Alarms() ([]core.Alarm, error) {
	srv, err := s.running()
	if err != nil {
		return nil, err
	}
	return srv.Alarms(), nil
}

// SetAlarm raises or clears the manual alarm of a point.
func (s *ServerService) SetAlarm(point string, active bool, severity int, message string) error {
	srv, err := s.running()
	if err != nil {
		return err
	}
	return srv.SetAlarm(strings.TrimSpace(point), active, uint16(severity), message)
}

// Acknowledge acknowledges an alarm.
func (s *ServerService) Acknowledge(source, name string) error {
	srv, err := s.running()
	if err != nil {
		return err
	}
	return srv.Acknowledge(source, name, "acknowledged in uatester")
}

// FireEvent sends a plain event.
func (s *ServerService) FireEvent(message string) error {
	srv, err := s.running()
	if err != nil {
		return err
	}
	return srv.FireEvent(message, 300)
}

// Rejected lists rejected client certificates.
func (s *ServerService) Rejected() ([]core.Cert, error) {
	srv, err := s.running()
	if err != nil {
		return nil, err
	}
	return srv.Rejected()
}

// Trust trusts a rejected client certificate.
func (s *ServerService) Trust(thumbprint string) error {
	srv, err := s.running()
	if err != nil {
		return err
	}
	return srv.Trust(thumbprint)
}

// ServiceShutdown stops the server when the window closes.
func (s *ServerService) ServiceShutdown() error { return s.Stop() }
