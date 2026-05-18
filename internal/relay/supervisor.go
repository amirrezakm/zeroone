package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/events"
	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
)

// Supervisor runs the mhrv-rs binary as a child process, writes its config
// before each start, restarts on crash with backoff, and periodically
// probes the listen port to publish health.
type Supervisor struct {
	ConfigPath string // path to stack.json (so we can hot-reload relay cfg)
	LoadConfig func() (stack.RelayConfig, error)
	Store      *Store
	Events     *events.Broker
	ProbeEvery time.Duration

	mu        sync.Mutex
	cmd       *exec.Cmd
	exited    chan struct{} // closed when the current child exits
	wantStop  bool
	restarts  int
	startedAt time.Time

	// reload wakes manageChild so it picks up a new config (start/stop the
	// child). probeNow wakes the probe loop. They are separate channels
	// because manageChild and Run both need to react to a config change —
	// a single shared channel races on buffer=1 and the loser misses the
	// edge.
	reload   chan struct{}
	probeNow chan struct{}
}

func NewSupervisor(configPath string, loader func() (stack.RelayConfig, error), store *Store, broker *events.Broker) *Supervisor {
	return &Supervisor{
		ConfigPath: configPath,
		LoadConfig: loader,
		Store:      store,
		Events:     broker,
		ProbeEvery: 15 * time.Second,
		reload:     make(chan struct{}, 1),
		probeNow:   make(chan struct{}, 1),
	}
}

// Reload signals the supervisor that the relay config on disk has changed.
// Both the child-management loop and the probe loop are woken — they have
// separate channels because a single buffered channel races between them.
func (s *Supervisor) Reload() {
	for _, ch := range []chan struct{}{s.reload, s.probeNow} {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (s *Supervisor) Run(ctx context.Context) {
	if s.LoadConfig == nil {
		slog.Error("relay supervisor missing LoadConfig")
		return
	}
	probeTicker := time.NewTicker(s.ProbeEvery)
	defer probeTicker.Stop()

	// Run loop: bring up child if enabled, monitor, restart on crash, reload
	// on signal, probe periodically.
	go s.manageChild(ctx)

	for {
		select {
		case <-ctx.Done():
			s.stopChild()
			return
		case <-probeTicker.C:
			s.runProbe(ctx)
		case <-s.probeNow:
			s.runProbe(ctx)
		}
	}
}

func (s *Supervisor) manageChild(ctx context.Context) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		cfg, err := s.LoadConfig()
		if err != nil {
			s.log("error", "load config", err)
			time.Sleep(5 * time.Second)
			continue
		}
		if !cfg.Enabled {
			s.stopChild()
			s.publishStatus(cfg, false)
			s.waitReloadOrCancel(ctx, 5*time.Second)
			continue
		}
		// systemd-managed mode: we never spawn the child, just report state.
		if cfg.SystemdUnit != "" {
			s.publishStatus(cfg, false)
			s.waitReloadOrCancel(ctx, 5*time.Second)
			continue
		}
		binary, configPath, _, _, _ := EffectivePaths(cfg)
		if _, err := os.Stat(binary); err != nil {
			s.log("error", "relay binary not found at "+binary, err)
			s.publishStatus(cfg, false)
			s.waitReloadOrCancel(ctx, 30*time.Second)
			continue
		}
		if err := WriteConfig(cfg, configPath); err != nil {
			s.log("error", "render mhrv-rs config", err)
			s.publishStatus(cfg, false)
			s.waitReloadOrCancel(ctx, 10*time.Second)
			continue
		}
		if err := s.startChild(binary, configPath, cfg); err != nil {
			s.log("error", "start relay", err)
			s.publishStatus(cfg, false)
			s.waitReloadOrCancel(ctx, backoff)
			if backoff < 60*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		s.publishStatus(cfg, true)
		// block until the child exits or a reload is requested
		s.waitChildOrReload(ctx)
	}
}

func (s *Supervisor) startChild(binary, configPath string, cfg stack.RelayConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd != nil && s.cmd.Process != nil {
		return nil // already running
	}
	args := []string{"--config", configPath, "--no-cert-check"}
	cmd := exec.Command(binary, args...)
	cmd.Env = os.Environ()
	logPath := cfg.LogPath
	if logPath == "" {
		logPath = stack.DefaultRelayLogPath
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	cmd.Stdout = lf
	cmd.Stderr = lf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		_ = lf.Close()
		return err
	}
	s.cmd = cmd
	s.startedAt = time.Now()
	s.exited = make(chan struct{})
	exited := s.exited
	s.log("info", fmt.Sprintf("relay started pid=%d", cmd.Process.Pid), nil)
	// One Wait per Cmd: only this goroutine calls Wait. stopChild observes
	// exit via the `exited` channel rather than calling Wait again (which
	// would return instantly with "Wait already called" and falsely report
	// the process as gone).
	go func() {
		_ = cmd.Wait()
		_ = lf.Close()
		s.mu.Lock()
		wasWanted := s.wantStop
		s.cmd = nil
		s.restarts++
		s.mu.Unlock()
		close(exited)
		if !wasWanted {
			s.log("warn", "relay exited", nil)
		}
	}()
	return nil
}

func (s *Supervisor) stopChild() {
	s.mu.Lock()
	cmd := s.cmd
	exited := s.exited
	s.wantStop = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.wantStop = false
		s.mu.Unlock()
	}()
	if cmd == nil || cmd.Process == nil || exited == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-exited:
		s.log("info", "relay stopped", nil)
		return
	case <-time.After(5 * time.Second):
	}
	// Still alive — mhrv-rs ignored SIGTERM (it does not install a graceful
	// signal handler). Escalate.
	_ = cmd.Process.Kill()
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		s.log("warn", "relay did not exit after SIGKILL", nil)
	}
	s.log("info", "relay killed", nil)
}

// Restart forces the child to be torn down and re-spawned.
func (s *Supervisor) Restart() {
	s.stopChild()
	s.Reload()
}

func (s *Supervisor) waitChildOrReload(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.reload:
			s.stopChild()
			return
		case <-t.C:
			s.mu.Lock()
			alive := s.cmd != nil
			s.mu.Unlock()
			if !alive {
				return
			}
		}
	}
}

func (s *Supervisor) waitReloadOrCancel(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-s.reload:
	case <-time.After(d):
	}
}

func (s *Supervisor) currentPID() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd != nil && s.cmd.Process != nil {
		return s.cmd.Process.Pid
	}
	return 0
}

// publishStatus refreshes the status without running an end-to-end probe.
func (s *Supervisor) publishStatus(cfg stack.RelayConfig, managed bool) {
	binary, configPath, statePath, _, _ := EffectivePaths(cfg)
	_, statErr := os.Stat(binary)
	st := Status{
		Enabled:      cfg.Enabled,
		Managed:      managed,
		Listen:       cfg.EffectiveListen(),
		OutboundTag:  cfg.EffectiveOutboundTag(),
		BinaryPath:   binary,
		ConfigPath:   configPath,
		SystemdUnit:  cfg.SystemdUnit,
		BinaryFound:  statErr == nil,
		EnabledSites: len(cfg.EnabledSites()),
		TotalSites:   len(cfg.Sites),
		PID:          s.currentPID(),
		Running:      s.currentPID() != 0,
		RestartCount: s.restarts,
		StartedAt:    s.startedAt,
	}
	s.Store.ReplaceStatus(st)
	_ = s.Store.Persist(statePath)
	if s.Events != nil {
		s.Events.Publish("relay.status", map[string]any{"enabled": st.Enabled, "running": st.Running, "listen": st.Listen})
	}
}

func (s *Supervisor) runProbe(ctx context.Context) {
	cfg, err := s.LoadConfig()
	if err != nil {
		s.log("error", "probe load config", err)
		return
	}
	if !cfg.Enabled {
		s.publishStatus(cfg, false)
		return
	}
	result := Probe(ctx, cfg, 8*time.Second)
	_, _, statePath, _, _ := EffectivePaths(cfg)
	s.Store.UpdateStatus(func(st *Status) {
		st.Enabled = cfg.Enabled
		st.Listen = cfg.EffectiveListen()
		st.OutboundTag = cfg.EffectiveOutboundTag()
		st.PID = s.currentPID()
		st.Running = st.PID != 0
		st.ListenOK = result.ListenOK
		st.HealthOK = result.HealthOK
		st.LastProbe = time.Now()
		st.LatencyMS = result.LatencyMS
		st.EnabledSites = len(cfg.EnabledSites())
		st.TotalSites = len(cfg.Sites)
		st.RestartCount = s.restarts
		if result.Error != "" {
			st.LastError = result.Error
		} else if result.OK {
			st.LastError = ""
		}
	})
	_ = s.Store.Persist(statePath)
	if !result.OK {
		s.log("warn", "probe failed", errors.New(result.Error))
	}
	if s.Events != nil {
		s.Events.Publish("relay.probe", map[string]any{
			"ok": result.OK, "listen_ok": result.ListenOK, "health_ok": result.HealthOK,
			"latency_ms": result.LatencyMS, "error": result.Error,
		})
	}
}

func (s *Supervisor) log(level, msg string, err error) {
	s.Store.AppendLog(level, msg, err)
	switch level {
	case "error":
		slog.Error("relay: "+msg, "err", err)
	case "warn":
		slog.Warn("relay: "+msg, "err", err)
	default:
		slog.Info("relay: " + msg)
	}
	if s.Events != nil {
		ev := map[string]any{"level": level, "msg": msg}
		if err != nil {
			ev["err"] = err.Error()
		}
		s.Events.Publish("relay.log", ev)
	}
}

// Tail returns the last N lines of the relay log file (best-effort).
func Tail(path string, max int) ([]string, error) {
	if path == "" {
		path = stack.DefaultRelayLogPath
	}
	if max <= 0 {
		max = 200
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, err
	}
	defer f.Close()
	// Read from end up to ~256KB to keep this cheap.
	const window = 256 * 1024
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	var off int64
	size := st.Size()
	if size > window {
		off = size - window
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return nil, err
	}
	buf, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	lines := splitLines(string(buf))
	if len(lines) > max {
		lines = lines[len(lines)-max:]
	}
	return lines, nil
}

func splitLines(s string) []string {
	out := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
