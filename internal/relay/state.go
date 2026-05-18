// Package relay manages the MasterHttpRelayVPN (mhrv-rs) plugin: rendering
// its config, supervising the binary, and exposing health to the panel.
package relay

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
)

// Status is the latest observed runtime state of the relay plugin.
type Status struct {
	Enabled       bool      `json:"enabled"`
	Managed       bool      `json:"managed"`
	Listen        string    `json:"listen"`
	OutboundTag   string    `json:"outbound_tag"`
	BinaryPath    string    `json:"binary_path"`
	ConfigPath    string    `json:"config_path"`
	SystemdUnit   string    `json:"systemd_unit,omitempty"`
	PID           int       `json:"pid,omitempty"`
	Running       bool      `json:"running"`
	ListenOK      bool      `json:"listen_ok"`
	HealthOK      bool      `json:"health_ok"`
	LastProbe     time.Time `json:"last_probe,omitempty"`
	LastError     string    `json:"last_error,omitempty"`
	LatencyMS     int64     `json:"latency_ms,omitempty"`
	BinaryFound   bool      `json:"binary_found"`
	EnabledSites  int       `json:"enabled_sites"`
	TotalSites    int       `json:"total_sites"`
	RestartCount  int       `json:"restart_count"`
	StartedAt     time.Time `json:"started_at,omitempty"`
	GeneratedAt   time.Time `json:"generated_at"`
}

// LogEntry is one structured event from the supervisor (process start,
// crash, probe failure, etc.) shown in the panel.
type LogEntry struct {
	T     int64  `json:"t"`
	Level string `json:"level"`
	Msg   string `json:"msg"`
	Err   string `json:"err,omitempty"`
}

// Store is the in-memory view of relay status + recent log buffer the
// supervisor publishes and the API reads.
type Store struct {
	mu      sync.RWMutex
	status  Status
	logs    []LogEntry
	maxLogs int
}

func NewStore() *Store {
	return &Store{maxLogs: 200}
}

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

// Persist writes the runtime status to disk so the panel can read a
// last-known snapshot even when the supervisor is offline.
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

// LoadStatus reads a persisted status from disk. Returns zero value if the
// file is missing.
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
func EffectivePaths(c stack.RelayConfig) (binary, configPath, statePath, logPath, healthProbe string) {
	binary = c.Binary
	if binary == "" {
		binary = stack.DefaultRelayBinary
	}
	configPath = c.ConfigPath
	if configPath == "" {
		configPath = stack.DefaultRelayConfigPath
	}
	statePath = c.StatePath
	if statePath == "" {
		statePath = stack.DefaultRelayStatePath
	}
	logPath = c.LogPath
	if logPath == "" {
		logPath = stack.DefaultRelayLogPath
	}
	healthProbe = c.HealthProbe
	if healthProbe == "" {
		healthProbe = stack.DefaultRelayHealthProbe
	}
	return
}
