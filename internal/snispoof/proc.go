package snispoof

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// managedProc is a single supervised child process (byedpi or tun2socks).
// It mirrors the relay supervisor's start/stop discipline: one Wait per Cmd,
// exit observed via a channel so stop never double-Waits.
type managedProc struct {
	name string

	mu        sync.Mutex
	cmd       *exec.Cmd
	exited    chan struct{}
	wantStop  bool
	restarts  int
	startedAt time.Time
}

func newProc(name string) *managedProc { return &managedProc{name: name} }

// start spawns the binary, appending stdout/stderr to logPath. It is a no-op
// if the process is already running. logf records lifecycle events.
func (p *managedProc) start(binary string, args []string, logPath string, logf func(level, msg string, err error)) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	cmd := exec.Command(binary, args...)
	cmd.Env = os.Environ()
	cmd.Stdout = lf
	cmd.Stderr = lf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		_ = lf.Close()
		return err
	}
	p.cmd = cmd
	p.startedAt = time.Now()
	p.exited = make(chan struct{})
	exited := p.exited
	if logf != nil {
		logf("info", fmt.Sprintf("%s started pid=%d", p.name, cmd.Process.Pid), nil)
	}
	go func() {
		_ = cmd.Wait()
		_ = lf.Close()
		p.mu.Lock()
		wasWanted := p.wantStop
		p.cmd = nil
		p.restarts++
		p.mu.Unlock()
		close(exited)
		if !wasWanted && logf != nil {
			logf("warn", p.name+" exited", nil)
		}
	}()
	return nil
}

// stop sends SIGTERM, then SIGKILL after 5s, and waits for exit.
func (p *managedProc) stop(logf func(level, msg string, err error)) {
	p.mu.Lock()
	cmd := p.cmd
	exited := p.exited
	p.wantStop = true
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.wantStop = false
		p.mu.Unlock()
	}()
	if cmd == nil || cmd.Process == nil || exited == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-exited:
		return
	case <-time.After(5 * time.Second):
	}
	_ = cmd.Process.Kill()
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		if logf != nil {
			logf("warn", p.name+" did not exit after SIGKILL", nil)
		}
	}
}

func (p *managedProc) pid() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

func (p *managedProc) alive() bool { return p.pid() != 0 }

// exitedCh returns the channel closed when the current process exits, or a
// nil channel when nothing is running (nil channels block forever in select,
// which is the desired behaviour).
func (p *managedProc) exitedCh() <-chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exited
}

func (p *managedProc) restartCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.restarts
}

func (p *managedProc) startedAtTime() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.startedAt
}
