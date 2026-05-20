package main

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
)

// migrateLegacyStateDir relocates the pre-rebrand /var/lib/xray-stack
// directory to /var/lib/zeroone on first run. It acts when the new
// path is the active state dir AND the new directory is either absent
// or empty — so an install flow that pre-creates /var/lib/zeroone with
// `mkdir -p` still picks up legacy state. Cross-device sources (legacy
// state living on a bind mount or separate volume) fall back to a
// recursive copy. Any failure is logged and ignored so the daemon
// still starts.
func migrateLegacyStateDir(stateDir string) {
	if stateDir != "/var/lib/zeroone" {
		return
	}
	legacy := "/var/lib/xray-stack"
	legacyInfo, err := os.Stat(legacy)
	if err != nil || !legacyInfo.IsDir() {
		return
	}

	switch newEntries, err := os.ReadDir(stateDir); {
	case os.IsNotExist(err):
		if err := os.MkdirAll(filepath.Dir(stateDir), 0o755); err != nil {
			slog.Warn("migrate legacy state dir: mkdir parent", "err", err)
			return
		}
		if err := moveTree(legacy, stateDir); err != nil {
			slog.Warn("migrate legacy state dir", "from", legacy, "to", stateDir, "err", err)
			return
		}
		slog.Warn("migrated legacy state dir", "from", legacy, "to", stateDir)
	case err != nil:
		slog.Warn("migrate legacy state dir: stat target", "path", stateDir, "err", err)
	case len(newEntries) == 0:
		legacyEntries, lerr := os.ReadDir(legacy)
		if lerr != nil {
			slog.Warn("migrate legacy state dir: read source", "path", legacy, "err", lerr)
			return
		}
		moved := 0
		for _, e := range legacyEntries {
			src := filepath.Join(legacy, e.Name())
			dst := filepath.Join(stateDir, e.Name())
			if err := moveTree(src, dst); err != nil {
				slog.Warn("migrate legacy state dir: move child", "src", src, "dst", dst, "err", err)
				continue
			}
			moved++
		}
		if moved > 0 {
			slog.Warn("migrated legacy state dir contents", "from", legacy, "to", stateDir, "entries", moved)
		}
		if err := os.Remove(legacy); err != nil && !os.IsNotExist(err) {
			// Non-fatal: legacy may still hold a failed entry or be a
			// mount point that can't be removed.
			slog.Warn("migrate legacy state dir: remove source", "path", legacy, "err", err)
		}
	}
}

// moveTree renames src to dst; on EXDEV (cross-filesystem) it falls
// back to a recursive copy followed by removal of the source.
func moveTree(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return err
	}
	if err := copyTree(src, dst); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

func copyTree(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, dst)
	case info.IsDir():
		if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyTree(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
				return err
			}
		}
		return nil
	case info.Mode().IsRegular():
		return copyFile(src, dst, info.Mode().Perm())
	default:
		// Skip devices, sockets, fifos — they should not appear under
		// the state dir, and we don't have a reasonable way to recreate
		// them without elevated assumptions.
		return nil
	}
}

func copyFile(src, dst string, mode os.FileMode) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}()
	_, err = io.Copy(out, in)
	return err
}
