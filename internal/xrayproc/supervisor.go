// SPDX-License-Identifier: AGPL-3.0-or-later

// Package xrayproc supervises a child xray process. It exists so the
// daemon can manage xray directly inside a container, replacing the
// `systemctl restart xray.service` path used on systemd hosts.
//
// The supervisor:
//   - Starts xray as `xray run -c <config>` and streams stdout/stderr
//     to a log file.
//   - Auto-restarts xray on crash with exponential backoff (capped at
//     60s) until ctx is canceled.
//   - Provides Restart() for external callers (the xray.Restarter
//     contract) that performs a SIGTERM-grace-SIGKILL cycle followed
//     by a respawn.
package xrayproc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/amirrezakm/zeroone/internal/system"
)

// Supervisor runs a single xray process and restarts it on demand.
type Supervisor struct {
	BinaryPath string
	ConfigPath string
	LogPath    string
	Logger     *slog.Logger

	mu           sync.Mutex
	cmd          *exec.Cmd
	logFile      *os.File
	restartCount int
	lastExit     error
	stopping     bool
}

// New constructs a Supervisor.
func New(binaryPath, configPath, logPath string, logger *slog.Logger) *Supervisor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Supervisor{
		BinaryPath: binaryPath,
		ConfigPath: configPath,
		LogPath:    logPath,
		Logger:     logger,
	}
}

// Run is the lifecycle loop. It starts xray, waits for it to exit, and
// either respawns (with backoff) or returns when ctx is done.
func (s *Supervisor) Run(ctx context.Context) {
	backoff := time.Second
	for {
		if err := ctx.Err(); err != nil {
			s.terminate(5 * time.Second)
			return
		}
		if err := s.start(); err != nil {
			s.Logger.Error("xrayproc: start failed", "err", err)
			if !sleepCtx(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}
		s.Logger.Info("xrayproc: xray started", "pid", s.PID(), "restart", s.RestartCount())

		exit := make(chan error, 1)
		go func(c *exec.Cmd) { exit <- c.Wait() }(s.currentCmd())

		var err error
		select {
		case <-ctx.Done():
			s.terminate(5 * time.Second)
			// drain
			select {
			case <-exit:
			case <-time.After(5 * time.Second):
			}
			return
		case err = <-exit:
		}

		s.mu.Lock()
		stopping := s.stopping
		s.stopping = false
		s.lastExit = err
		s.cmd = nil
		s.mu.Unlock()

		if ctx.Err() != nil {
			return
		}
		if stopping {
			backoff = time.Second
			continue
		}
		s.Logger.Warn("xrayproc: xray exited", "err", err, "next_restart_in", backoff)
		if !sleepCtx(ctx, backoff) {
			return
		}
		backoff = nextBackoff(backoff)
	}
}

// Restart implements the xray.Restarter contract. It signals the
// running process; the Run loop respawns it.
func (s *Supervisor) Restart(ctx context.Context, _ system.Runner) error {
	s.mu.Lock()
	cmd := s.cmd
	s.stopping = true
	s.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return errors.New("xrayproc: not running")
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		s.Logger.Warn("xrayproc: SIGTERM failed", "err", err)
	}
	// Best-effort wait for the process to terminate. The Run loop will
	// respawn it. We don't call cmd.Wait() here — that's owned by Run.
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			_ = cmd.Process.Kill()
			return nil
		case <-tick.C:
			if s.currentCmd() != cmd {
				return nil
			}
		}
	}
}

// terminate sends SIGTERM, waits up to grace, then SIGKILLs.
func (s *Supervisor) terminate(grace time.Duration) {
	cmd := s.currentCmd()
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if cmd.ProcessState != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
}

// Status returns a snapshot of current process state.
type Status struct {
	PID          int
	RestartCount int
	LastExit     string
}

func (s *Supervisor) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{PID: s.pidLocked(), RestartCount: s.restartCount}
	if s.lastExit != nil {
		st.LastExit = s.lastExit.Error()
	}
	return st
}

func (s *Supervisor) PID() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pidLocked()
}

func (s *Supervisor) RestartCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.restartCount
}

func (s *Supervisor) currentCmd() *exec.Cmd {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cmd
}

func (s *Supervisor) pidLocked() int {
	if s.cmd != nil && s.cmd.Process != nil {
		return s.cmd.Process.Pid
	}
	return 0
}

func (s *Supervisor) start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd != nil {
		return errors.New("already running")
	}
	if s.LogPath != "" {
		if err := os.MkdirAll(filepath.Dir(s.LogPath), 0o755); err != nil {
			return err
		}
	}
	var logW io.Writer = io.Discard
	if s.LogPath != "" {
		f, err := os.OpenFile(s.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
		if err != nil {
			return err
		}
		if s.logFile != nil {
			_ = s.logFile.Close()
		}
		s.logFile = f
		logW = f
	}
	cmd := exec.Command(s.BinaryPath, "run", "-c", s.ConfigPath)
	cmd.Stdout = logW
	cmd.Stderr = logW
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	s.cmd = cmd
	s.restartCount++
	return nil
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func nextBackoff(cur time.Duration) time.Duration {
	next := cur * 2
	if next > 60*time.Second {
		next = 60 * time.Second
	}
	return next
}
