// Package snapshots captures a stack.json + xray config.json pair
// before each apply so a rollback is always one click away.
package snapshots

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Source values for Info.Source. Manual snapshots are operator-initiated
// and kept forever; auto snapshots are taken automatically before risky
// mutations and pruned past a cap.
const (
	SourceManual = "manual"
	SourceAuto   = "auto"
)

type Info struct {
	ID        string `json:"id"`
	Time      int64  `json:"t"`
	Title     string `json:"title,omitempty"`
	Source    string `json:"source,omitempty"`
	Action    string `json:"action,omitempty"`
	StackPath string `json:"stack_path"`
	XrayPath  string `json:"xray_path"`
}

type Store struct {
	dir string
}

func New(dir string) *Store { return &Store{dir: dir} }

// Capture writes a snapshot of stackSrc and xraySrc into a new directory
// named with a UTC timestamp, and persists Title/Source/Action from meta
// in a sidecar meta.json so the panel can render them later.
func (s *Store) Capture(stackSrc, xraySrc string, meta Info) (Info, error) {
	if s.dir == "" {
		return Info{}, errors.New("snapshot dir not configured")
	}
	id := time.Now().UTC().Format("20060102-150405")
	dir := filepath.Join(s.dir, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Info{}, err
	}
	stackDst := filepath.Join(dir, "stack.json")
	xrayDst := filepath.Join(dir, "xray.json")
	if err := copyFile(stackSrc, stackDst, 0o600); err != nil {
		return Info{}, err
	}
	if err := copyFile(xraySrc, xrayDst, 0o600); err != nil {
		return Info{}, err
	}
	info := Info{
		ID:        id,
		Time:      time.Now().Unix(),
		Title:     strings.TrimSpace(meta.Title),
		Source:    meta.Source,
		Action:    meta.Action,
		StackPath: stackDst,
		XrayPath:  xrayDst,
	}
	if err := writeMeta(dir, info); err != nil {
		return Info{}, err
	}
	return info, nil
}

func (s *Store) List() ([]Info, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Info{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]Info, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		ts, err := time.Parse("20060102-150405", e.Name())
		if err != nil {
			continue
		}
		info := Info{
			ID:        e.Name(),
			Time:      ts.Unix(),
			StackPath: filepath.Join(s.dir, e.Name(), "stack.json"),
			XrayPath:  filepath.Join(s.dir, e.Name(), "xray.json"),
		}
		// Backward-compat: snapshots taken before the meta sidecar existed
		// have no meta.json. Surface them with empty title/source so the
		// panel still lists them; prune treats them as manual.
		if m, err := readMeta(filepath.Join(s.dir, e.Name())); err == nil {
			info.Title = m.Title
			info.Source = m.Source
			info.Action = m.Action
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time > out[j].Time })
	return out, nil
}

// Has reports whether a snapshot directory exists. Uses the same id
// sanity check as Rollback so callers get a clean false (rather than a
// path-escape) on malformed input.
func (s *Store) Has(id string) bool {
	if id == "" || strings.Contains(id, "/") || strings.Contains(id, "..") {
		return false
	}
	info, err := os.Stat(filepath.Join(s.dir, id))
	return err == nil && info.IsDir()
}

// Rollback overwrites stackDst with the snapshot's stack.json and
// xrayDst with the snapshot's xray.json. The caller is responsible for
// reloading services / re-running apply afterwards.
func (s *Store) Rollback(id, stackDst, xrayDst string) error {
	if strings.Contains(id, "/") || strings.Contains(id, "..") {
		return errors.New("invalid snapshot id")
	}
	src := filepath.Join(s.dir, id)
	if _, err := os.Stat(src); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(src, "stack.json"), stackDst, 0o600); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(src, "xray.json"), xrayDst, 0o600); err != nil {
		return err
	}
	return nil
}

// Prune deletes the oldest auto snapshots when their count exceeds
// maxAuto. Manual (and legacy meta-less) snapshots are never removed.
// Returns the IDs that were deleted.
func (s *Store) Prune(maxAuto int) ([]string, error) {
	if maxAuto <= 0 {
		return nil, nil
	}
	list, err := s.List()
	if err != nil {
		return nil, err
	}
	// list is newest-first; collect auto IDs in newest-first order and
	// remove the tail (oldest) past the cap.
	autos := make([]Info, 0, len(list))
	for _, info := range list {
		if info.Source == SourceAuto {
			autos = append(autos, info)
		}
	}
	if len(autos) <= maxAuto {
		return nil, nil
	}
	var removed []string
	for _, info := range autos[maxAuto:] {
		if err := os.RemoveAll(filepath.Join(s.dir, info.ID)); err != nil {
			return removed, err
		}
		removed = append(removed, info.ID)
	}
	return removed, nil
}

func writeMeta(dir string, info Info) error {
	meta := struct {
		Title  string `json:"title,omitempty"`
		Source string `json:"source,omitempty"`
		Action string `json:"action,omitempty"`
		Time   int64  `json:"t"`
	}{info.Title, info.Source, info.Action, info.Time}
	buf, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "meta.json"), buf, 0o600)
}

func readMeta(dir string) (Info, error) {
	buf, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return Info{}, err
	}
	var meta struct {
		Title  string `json:"title,omitempty"`
		Source string `json:"source,omitempty"`
		Action string `json:"action,omitempty"`
	}
	if err := json.Unmarshal(buf, &meta); err != nil {
		return Info{}, err
	}
	return Info{Title: meta.Title, Source: meta.Source, Action: meta.Action}, nil
}

func copyFile(src, dst string, mode os.FileMode) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	// The destination is writable, so a Close error can mean buffered data
	// never reached disk — surface it instead of dropping it. The named
	// return lets the deferred Close report a flush failure when the copy
	// itself succeeded.
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
