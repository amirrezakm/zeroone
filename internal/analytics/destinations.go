// Package analytics aggregates per-destination request counts from xray's
// journal output. Persisted as hourly buckets with 48 h retention.
package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Defaults tuned for a typical xray-stack deployment.
const (
	DefaultRetention = 48 * time.Hour
	DefaultTickEvery = 60 * time.Second
)

// HourBucket holds counts for a single hour. Hour is the unix timestamp at
// the start of the bucket (truncated to the hour).
type HourBucket struct {
	Hour         int64            `json:"hour"`
	Destinations map[string]int64 `json:"destinations"`
	Total        int64            `json:"total"`
}

type State struct {
	Buckets    []HourBucket `json:"buckets"`
	LastCursor string       `json:"last_cursor,omitempty"`
	UpdatedAt  int64        `json:"updated_at"`
}

// Top returns the top-n destinations summed across the live buckets.
type Item struct {
	Destination string `json:"destination"`
	Requests    int64  `json:"requests"`
}

type Result struct {
	Items     []Item `json:"items"`
	Total     int64  `json:"total"`
	Window    string `json:"window"`
	UpdatedAt int64  `json:"updated_at"`
}

// Aggregator parses xray journal output into hourly destination counters.
// All access goes through Snapshot/Top so the read path is lock-protected.
type Aggregator struct {
	path      string
	unit      string
	retention time.Duration

	mu    sync.Mutex
	state State
}

func New(path string) *Aggregator {
	return &Aggregator{path: path, unit: "xray", retention: DefaultRetention}
}

func (a *Aggregator) load() {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, err := os.ReadFile(a.path)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		slog.Warn("destinations load", "err", err)
		return
	}
	if len(b) == 0 {
		return
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		slog.Warn("destinations parse", "err", err)
		return
	}
	a.state = st
}

func (a *Aggregator) save() {
	if a.path == "" {
		return
	}
	a.state.UpdatedAt = time.Now().Unix()
	b, err := json.Marshal(a.state)
	if err != nil {
		slog.Warn("destinations marshal", "err", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(a.path), 0o755); err != nil {
		slog.Warn("destinations mkdir", "err", err)
		return
	}
	tmp := a.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		slog.Warn("destinations write", "err", err)
		return
	}
	if err := os.Rename(tmp, a.path); err != nil {
		slog.Warn("destinations rename", "err", err)
	}
}

// Run blocks until ctx is done, ticking every DefaultTickEvery.
func (a *Aggregator) Run(ctx context.Context) {
	if a.path == "" {
		slog.Info("destinations path not set; aggregator disabled")
		return
	}
	a.load()
	a.tick(ctx) // prime
	t := time.NewTicker(DefaultTickEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.tick(ctx)
		}
	}
}

// acceptedLine pulls (proto, dest:port, tag-pair) from a typical xray log
// line: "from <client> accepted tcp:<dst>:<port> [in -> out] email: foo".
var acceptedLine = regexp.MustCompile(`accepted (?:tcp|udp):([^\s]+) \[([^\]]+)\]`)
var cursorLine = regexp.MustCompile(`^-- cursor: (.+)$`)

func (a *Aggregator) tick(ctx context.Context) {
	args := []string{"-u", a.unit, "--no-pager", "--output=short-iso", "--show-cursor"}
	a.mu.Lock()
	if a.state.LastCursor != "" {
		args = append(args, "--after-cursor="+a.state.LastCursor)
	} else {
		// First run: pull last 5 minutes so the panel has something immediately.
		args = append(args, "--since=5 minutes ago")
	}
	a.mu.Unlock()
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "journalctl", args...)
	out, err := cmd.Output()
	if err != nil {
		slog.Debug("destinations journalctl", "err", err)
		return
	}
	a.ingest(string(out))
}

func (a *Aggregator) ingest(text string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cutoff := time.Now().Add(-a.retention).Unix()
	now := time.Now().Unix()
	currentHour := now - (now % 3600)
	bucket := a.bucketRef(currentHour)

	for _, line := range strings.Split(text, "\n") {
		if line == "" {
			continue
		}
		if m := cursorLine.FindStringSubmatch(line); len(m) == 2 {
			a.state.LastCursor = m[1]
			continue
		}
		if !strings.Contains(line, " accepted ") {
			continue
		}
		m := acceptedLine.FindStringSubmatch(line)
		if len(m) != 3 {
			continue
		}
		// m[1] = dest:port (or [v6]:port), m[2] = "<inbound> -> <outbound>" or ">>"
		dest := stripPort(m[1])
		if dest == "" {
			continue
		}
		bucket.Destinations[dest]++
		bucket.Total++
	}

	// Persist the current bucket back into the slice (bucketRef returned a copy
	// when creating new buckets — the slice already owns the modified map for
	// existing buckets, but rewriting is harmless).
	a.upsertBucket(*bucket)
	a.prune(cutoff)
	a.save()
}

// bucketRef returns a pointer to the bucket for the given hour, creating it
// if missing. Buckets are kept sorted by hour ascending.
func (a *Aggregator) bucketRef(hour int64) *HourBucket {
	for i := range a.state.Buckets {
		if a.state.Buckets[i].Hour == hour {
			if a.state.Buckets[i].Destinations == nil {
				a.state.Buckets[i].Destinations = map[string]int64{}
			}
			return &a.state.Buckets[i]
		}
	}
	a.state.Buckets = append(a.state.Buckets, HourBucket{Hour: hour, Destinations: map[string]int64{}})
	return &a.state.Buckets[len(a.state.Buckets)-1]
}

func (a *Aggregator) upsertBucket(b HourBucket) {
	for i := range a.state.Buckets {
		if a.state.Buckets[i].Hour == b.Hour {
			a.state.Buckets[i] = b
			return
		}
	}
	a.state.Buckets = append(a.state.Buckets, b)
}

func (a *Aggregator) prune(cutoff int64) {
	kept := a.state.Buckets[:0]
	for _, b := range a.state.Buckets {
		if b.Hour+3600 > cutoff {
			kept = append(kept, b)
		}
	}
	a.state.Buckets = kept
	sort.Slice(a.state.Buckets, func(i, j int) bool { return a.state.Buckets[i].Hour < a.state.Buckets[j].Hour })
}

// Top returns the top-n destinations summed across the live buckets within
// the configured retention window.
func (a *Aggregator) Top(limit int) Result {
	a.mu.Lock()
	defer a.mu.Unlock()
	if limit <= 0 {
		limit = 20
	}
	cutoff := time.Now().Add(-a.retention).Unix()
	agg := map[string]int64{}
	var total int64
	for _, b := range a.state.Buckets {
		if b.Hour+3600 <= cutoff {
			continue
		}
		for d, c := range b.Destinations {
			agg[d] += c
			total += c
		}
	}
	items := make([]Item, 0, len(agg))
	for d, c := range agg {
		items = append(items, Item{Destination: d, Requests: c})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Requests != items[j].Requests {
			return items[i].Requests > items[j].Requests
		}
		return items[i].Destination < items[j].Destination
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return Result{
		Items:     items,
		Total:     total,
		Window:    fmt.Sprintf("%dh", int(a.retention.Hours())),
		UpdatedAt: a.state.UpdatedAt,
	}
}

// stripPort turns "host:port" or "[v6]:port" into just "host"/"v6". Returns
// empty string for inputs that don't look like a host:port pair.
func stripPort(addr string) string {
	if addr == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	// Some lines may already be hostless; fall back to raw.
	return addr
}
