// Package presence persists "last seen" timestamps per panel user across
// zeroone restarts. A background ticker queries the journal every
// minute and updates the tracker so the panel can show "last seen 3h ago"
// even for users who are not currently online.
package presence

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/amirrezakm/zeroone/internal/monitor"
	"github.com/amirrezakm/zeroone/internal/stack"
	"github.com/amirrezakm/zeroone/internal/system"
)

type Entry struct {
	LastSeen int64  `json:"last_seen"`
	LastIP   string `json:"last_ip,omitempty"`
}

type Tracker struct {
	mu      sync.RWMutex
	path    string
	entries map[string]Entry
	dirty   bool
}

func New(path string) *Tracker {
	t := &Tracker{path: path, entries: map[string]Entry{}}
	t.load()
	return t
}

func (t *Tracker) load() {
	b, err := os.ReadFile(t.path)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		return
	}
	_ = json.Unmarshal(b, &t.entries)
}

func (t *Tracker) Snapshot() map[string]Entry {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make(map[string]Entry, len(t.entries))
	for k, v := range t.entries {
		out[k] = v
	}
	return out
}

func (t *Tracker) Touch(email string, lastSeen int64, lastIP string) {
	if email == "" {
		return
	}
	t.mu.Lock()
	cur := t.entries[email]
	if lastSeen > cur.LastSeen {
		cur.LastSeen = lastSeen
		if lastIP != "" {
			cur.LastIP = lastIP
		}
		t.entries[email] = cur
		t.dirty = true
	}
	t.mu.Unlock()
}

func (t *Tracker) flush() {
	t.mu.Lock()
	if !t.dirty {
		t.mu.Unlock()
		return
	}
	snap := make(map[string]Entry, len(t.entries))
	for k, v := range t.entries {
		snap[k] = v
	}
	t.dirty = false
	t.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(t.path), 0o755); err != nil {
		return
	}
	tmp := t.path + ".tmp"
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, t.path)
}

// Run polls Online() periodically and folds new observations into the
// tracker. On startup it does a wider one-shot scan so the tracker is
// pre-warmed with last_seen for users who connected hours ago.
// Returns when ctx is done.
func (t *Tracker) Run(ctx context.Context, configRead func() stack.Config) {
	tick := time.NewTicker(60 * time.Second)
	defer tick.Stop()
	flushTick := time.NewTicker(2 * time.Minute)
	defer flushTick.Stop()
	// Wide one-shot at startup (24h) — single journalctl call, slow but
	// only happens once per process start.
	t.poll(ctx, configRead, 24*3600)
	t.flush()
	for {
		select {
		case <-ctx.Done():
			t.flush()
			return
		case <-tick.C:
			t.poll(ctx, configRead, 90)
		case <-flushTick.C:
			t.flush()
		}
	}
}

func (t *Tracker) poll(ctx context.Context, configRead func() stack.Config, windowSeconds int) {
	cfg := configRead()
	ports := []int{}
	if p := cfg.Xray.Inbounds.VLESSWSPort; p != 0 {
		ports = append(ports, p)
	}
	for _, sock := range cfg.Xray.Inbounds.PublicSOCKS {
		if sock.Port != 0 {
			ports = append(ports, sock.Port)
		}
	}
	for _, u := range cfg.Xray.Users {
		if u.BandwidthPort != 0 {
			ports = append(ports, u.BandwidthPort)
		}
	}
	timeout := 6 * time.Second
	if windowSeconds > 600 {
		timeout = 30 * time.Second
	}
	snap, err := monitor.Online(ctx, system.ExecRunner{Timeout: timeout}, windowSeconds, ports)
	if err != nil {
		return
	}
	for _, u := range snap.Users {
		ip := ""
		if len(u.IPs) > 0 {
			ip = u.IPs[0]
		}
		t.Touch(u.Email, u.LastSeen, ip)
	}
}
