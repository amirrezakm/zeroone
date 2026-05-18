// Package snapshots captures a stack.json + xray config.json pair
// before each apply so a rollback is always one click away.
package snapshots

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Info struct {
	ID        string `json:"id"`
	Time      int64  `json:"t"`
	StackPath string `json:"stack_path"`
	XrayPath  string `json:"xray_path"`
}

type Store struct {
	dir string
}

func New(dir string) *Store { return &Store{dir: dir} }

// Capture writes a snapshot of stackSrc and xraySrc into a new directory
// named with a UTC timestamp. Returns the snapshot ID (directory basename).
func (s *Store) Capture(stackSrc, xraySrc string) (Info, error) {
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
	return Info{ID: id, Time: time.Now().Unix(), StackPath: stackDst, XrayPath: xrayDst}, nil
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
		out = append(out, Info{
			ID:        e.Name(),
			Time:      ts.Unix(),
			StackPath: filepath.Join(s.dir, e.Name(), "stack.json"),
			XrayPath:  filepath.Join(s.dir, e.Name(), "xray.json"),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time > out[j].Time })
	return out, nil
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

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
