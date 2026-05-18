// Package audit is an append-only JSON-line writer used to record
// every mutating panel action: who, when, what, and a small data blob.
// File format: one JSON object per line, newest at the end.
package audit

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Entry struct {
	Time   int64          `json:"t"`
	Actor  string         `json:"actor"`
	Action string         `json:"action"`
	Target string         `json:"target,omitempty"`
	Data   map[string]any `json:"data,omitempty"`
}

type Log struct {
	mu   sync.Mutex
	path string
}

func New(path string) *Log { return &Log{path: path} }

func (l *Log) Write(actor, action, target string, data map[string]any) error {
	if l.path == "" {
		return errors.New("audit log path not configured")
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	entry := Entry{Time: time.Now().Unix(), Actor: actor, Action: action, Target: target, Data: data}
	b, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

// Tail returns up to limit most-recent entries, newest first.
func (l *Log) Tail(limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = 100
	}
	f, err := os.Open(l.path)
	if errors.Is(err, os.ErrNotExist) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// For panel scale (audit log is small) a full scan is fine.
	br := bufio.NewReader(f)
	all := make([]Entry, 0, 256)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			var e Entry
			if json.Unmarshal(line[:len(line)-1], &e) == nil {
				all = append(all, e)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	if len(all) > limit {
		all = all[len(all)-limit:]
	}
	// reverse to newest-first
	out := make([]Entry, len(all))
	for i, e := range all {
		out[len(all)-1-i] = e
	}
	return out, nil
}
