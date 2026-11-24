package core

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/ka1hung/opcua/server"
	"github.com/ka1hung/opcua/ua"
)

// Version is this tool's version. Release builds set it from the git
// tag: -ldflags "-X github.com/ka1hung/uatester/core.Version=0.1.0".
var Version = "dev"

// history keeps the last n values per node and events in memory.
type history struct {
	mu     sync.Mutex
	n      int
	values map[string][]*ua.DataValue
	events []*server.Event
}

func newHistory(n int) *history { return &history{n: n, values: map[string][]*ua.DataValue{}} }

func (h *history) add(nid *ua.NodeID, dv *ua.DataValue) {
	h.mu.Lock()
	defer h.mu.Unlock()
	k := nid.String()
	vs := append(h.values[k], dv)
	if len(vs) > h.n {
		vs = vs[len(vs)-h.n:]
	}
	h.values[k] = vs
}

func (h *history) addEvent(e *server.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, e)
	if len(h.events) > h.n {
		h.events = h.events[len(h.events)-h.n:]
	}
}

func pos(c []byte) int {
	if len(c) == 0 {
		return 0
	}
	i, _ := strconv.Atoi(string(c))
	return i
}

func (h *history) ReadRaw(ctx context.Context, node *ua.NodeID, start, end time.Time, max int, bounds bool, c []byte) ([]*ua.DataValue, []byte, error) {
	h.mu.Lock()
	all := h.values[node.String()]
	h.mu.Unlock()
	lo, hi := start, end
	reverse := !start.IsZero() && !end.IsZero() && start.After(end)
	if reverse {
		lo, hi = end, start
	}
	var in []*ua.DataValue
	for _, v := range all {
		if (lo.IsZero() || !v.SourceTimestamp.Before(lo)) && (hi.IsZero() || !v.SourceTimestamp.After(hi)) {
			in = append(in, v)
		}
	}
	if reverse {
		sort.SliceStable(in, func(i, j int) bool { return in[i].SourceTimestamp.After(in[j].SourceTimestamp) })
	}
	from := pos(c)
	if from > len(in) {
		from = len(in)
	}
	if to := from + max; to < len(in) {
		return in[from:to], []byte(strconv.Itoa(to)), nil
	}
	return in[from:], nil, nil
}

func (h *history) ReadAtTime(ctx context.Context, node *ua.NodeID, times []time.Time) ([]*ua.DataValue, error) {
	h.mu.Lock()
	all := h.values[node.String()]
	h.mu.Unlock()
	out := make([]*ua.DataValue, len(times))
	for i, t := range times {
		j := sort.Search(len(all), func(k int) bool { return all[k].SourceTimestamp.After(t) })
		if j == 0 {
			out[i] = &ua.DataValue{Status: ua.StatusBadNoData, SourceTimestamp: t}
		} else {
			out[i] = all[j-1]
		}
	}
	return out, nil
}

func (h *history) ReadEvents(ctx context.Context, notifier *ua.NodeID, start, end time.Time, max int, c []byte) ([]*server.Event, []byte, error) {
	h.mu.Lock()
	all := append([]*server.Event(nil), h.events...)
	h.mu.Unlock()
	var in []*server.Event
	for _, e := range all {
		if (start.IsZero() || !e.Time.Before(start)) && (end.IsZero() || !e.Time.After(end)) {
			in = append(in, e)
		}
	}
	from := pos(c)
	if from > len(in) {
		from = len(in)
	}
	if to := from + max; to < len(in) {
		return in[from:to], []byte(strconv.Itoa(to)), nil
	}
	return in[from:], nil, nil
}
