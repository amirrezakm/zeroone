package failover

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Entry records a single failover transition as detected by the manager.
// Pending decisions (still gathering confirmations) are NOT recorded; only
// effective changes are appended to history.
type Entry struct {
	T      int64  `json:"t"`
	From   Mode   `json:"from"`
	To     Mode   `json:"to"`
	Reason string `json:"reason"`
	Error  string `json:"error,omitempty"`
}

const DefaultHistoryRetention = 48 * time.Hour

func LoadHistory(path string) ([]Entry, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []Entry
	if len(b) == 0 {
		return entries, nil
	}
	if err := json.Unmarshal(b, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// AppendHistory records a transition and prunes anything older than retention.
// Concurrent callers are expected to be sequenced by the caller (the failover
// manager runs single-threaded).
func AppendHistory(path string, entry Entry, retention time.Duration) error {
	if path == "" {
		return nil
	}
	if retention <= 0 {
		retention = DefaultHistoryRetention
	}
	entries, err := LoadHistory(path)
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-retention).Unix()
	pruned := entries[:0]
	for _, e := range entries {
		if e.T >= cutoff {
			pruned = append(pruned, e)
		}
	}
	pruned = append(pruned, entry)
	sort.Slice(pruned, func(i, j int) bool { return pruned[i].T < pruned[j].T })
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(pruned, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
