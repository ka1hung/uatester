package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// LogLine is one log message shown in the app.
type LogLine struct {
	Time  time.Time `json:"time"`
	Side  string    `json:"side"` // server or client
	Level string    `json:"level"`
	Text  string    `json:"text"`
}

// logHub keeps recent log lines and sends each new one to the window.
type logHub struct {
	mu    sync.Mutex
	lines []LogLine
	emit  func(name string, data any)
}

const keepLines = 500

func (h *logHub) add(l LogLine) {
	h.mu.Lock()
	h.lines = append(h.lines, l)
	if len(h.lines) > keepLines {
		h.lines = h.lines[len(h.lines)-keepLines:]
	}
	h.mu.Unlock()
	h.emit("log", l)
}

func (h *logHub) recent() []LogLine {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]LogLine(nil), h.lines...)
}

// logger returns a slog.Logger whose lines go to the hub, tagged side.
func (h *logHub) logger(side string) *slog.Logger {
	return slog.New(&hubHandler{hub: h, side: side})
}

func (h *logHub) printf(side, level, format string, args ...any) {
	h.add(LogLine{Time: time.Now(), Side: side, Level: level, Text: fmt.Sprintf(format, args...)})
}

type hubHandler struct {
	hub   *logHub
	side  string
	attrs []slog.Attr
}

func (h *hubHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= slog.LevelInfo }

func (h *hubHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Message)
	write := func(a slog.Attr) bool {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value.Any())
		return true
	}
	for _, a := range h.attrs {
		write(a)
	}
	r.Attrs(write)
	h.hub.add(LogLine{Time: r.Time, Side: h.side, Level: strings.ToLower(r.Level.String()), Text: b.String()})
	return nil
}

func (h *hubHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &hubHandler{hub: h.hub, side: h.side, attrs: append(append([]slog.Attr(nil), h.attrs...), attrs...)}
}

func (h *hubHandler) WithGroup(string) slog.Handler { return h }
