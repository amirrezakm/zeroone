// Package snispoof manages the SNI-spoofing / DPI-desync plugin. It
// supervises two child processes — byedpi (a local SOCKS5 proxy that
// applies the desync, e.g. injecting a fake ClientHello with a decoy SNI)
// and tun2socks (a real tun device backed by that proxy) — and programs a
// scoped policy route so xray's marked traffic flows through the tun while
// byedpi's own egress stays on the normal path. Mirrors internal/relay.
package snispoof

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/amirrezakm/zeroone/internal/stack"
)

// Status is the latest observed runtime state of the plugin.
type Status struct {
	Enabled          bool      `json:"enabled"`
	Managed          bool      `json:"managed"`
	Listen           string    `json:"listen"`
	OutboundTag      string    `json:"outbound_tag"`
	Method           string    `json:"method"`
	FakeDomain       string    `json:"fake_domain"`
	TunName          string    `json:"tun_name"`
	ByeDPIBinary     string    `json:"byedpi_binary"`
	Tun2socksBinary  string    `json:"tun2socks_binary"`
	ByeDPIPID        int       `json:"byedpi_pid,omitempty"`
	Tun2socksPID     int       `json:"tun2socks_pid,omitempty"`
	ByeDPIRunning    bool      `json:"byedpi_running"`
	Tun2socksRunning bool      `json:"tun2socks_running"`
	RouteReady       bool      `json:"route_ready"`
	BinariesFound    bool      `json:"binaries_found"`
	ListenOK         bool      `json:"listen_ok"`
	HealthOK         bool      `json:"health_ok"`
	LastProbe        time.Time `json:"last_probe,omitempty"`
	LastError        string    `json:"last_error,omitempty"`
	LatencyMS        int64     `json:"latency_ms,omitempty"`
	EnabledSites     int       `json:"enabled_sites"`
	TotalSites       int       `json:"total_sites"`
	RestartCount     int       `json:"restart_count"`
	StartedAt        time.Time `json:"started_at,omitempty"`
	GeneratedAt      time.Time `json:"generated_at"`
}

// LogEntry is one structured event from the supervisor shown in the panel.
type LogEntry struct {
	T     int64  `json:"t"`
	Level string `json:"level"`
	Msg   string `json:"msg"`
	Err   string `json:"err,omitempty"`
}

// Store is the in-memory view of status + recent log buffer.
type Store struct {
	mu      sync.RWMutex
	status  Status
	logs    []LogEntry
	maxLogs int
}

func NewStore() *Store { return &Store{maxLogs: 200} }

func (s *Store) Snapshot() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := s.status
	out.GeneratedAt = time.Now()
	return out
}

func (s *Store) Logs() []LogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]LogEntry, len(s.logs))
	copy(out, s.logs)
	return out
}

func (s *Store) UpdateStatus(fn func(*Status)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.status)
}

func (s *Store) ReplaceStatus(st Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st.GeneratedAt = time.Now()
	s.status = st
}

func (s *Store) AppendLog(level, msg string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := LogEntry{T: time.Now().Unix(), Level: level, Msg: msg}
	if err != nil {
		e.Err = err.Error()
	}
	s.logs = append(s.logs, e)
	if len(s.logs) > s.maxLogs {
		s.logs = s.logs[len(s.logs)-s.maxLogs:]
	}
}

// Persist writes the runtime status to disk for a last-known snapshot.
func (s *Store) Persist(path string) error {
	if path == "" {
		return nil
	}
	snap := s.Snapshot()
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadStatus reads a persisted status; zero value if the file is missing.
func LoadStatus(path string) (Status, error) {
	if path == "" {
		return Status{}, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Status{}, nil
		}
		return Status{}, err
	}
	var st Status
	if err := json.Unmarshal(b, &st); err != nil {
		return Status{}, err
	}
	return st, nil
}

// EffectivePaths fills in defaults so callers always have concrete paths.
func EffectivePaths(c stack.SNISpoofConfig) (byedpi, tun2socks, configDir, statePath, logPath, healthProbe string) {
	byedpi = c.Binary
	if byedpi == "" {
		byedpi = stack.DefaultSNISpoofBinary
	}
	tun2socks = c.Tun2socksBinary
	if tun2socks == "" {
		tun2socks = stack.DefaultSNISpoofTun2socks
	}
	configDir = c.ConfigDir
	if configDir == "" {
		configDir = stack.DefaultSNISpoofConfigDir
	}
	statePath = c.StatePath
	if statePath == "" {
		statePath = stack.DefaultSNISpoofStatePath
	}
	logPath = c.LogPath
	if logPath == "" {
		logPath = stack.DefaultSNISpoofLogPath
	}
	healthProbe = c.HealthProbe
	if healthProbe == "" {
		healthProbe = stack.DefaultSNISpoofHealthProbe
	}
	return
}
