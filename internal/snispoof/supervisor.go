package snispoof

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/amirrezakm/zeroone/internal/events"
	"github.com/amirrezakm/zeroone/internal/stack"
	"github.com/amirrezakm/zeroone/internal/system"
)

// Supervisor runs byedpi + tun2socks as child processes, programs the scoped
// policy route that steers xray's marked traffic through the tun, restarts on
// crash with backoff, and periodically probes the proxy to publish health.
type Supervisor struct {
	ConfigPath string
	LoadConfig func() (stack.SNISpoofConfig, error)
	LogLevel   func() string // tun2socks log level; optional
	Store      *Store
	Events     *events.Broker
	Runner     system.Runner
	ProbeEvery time.Duration

	byedpi    *managedProc
	tun2socks *managedProc

	mu         sync.Mutex
	routeReady bool

	reload   chan struct{}
	probeNow chan struct{}
}

func NewSupervisor(configPath string, loader func() (stack.SNISpoofConfig, error), store *Store, broker *events.Broker) *Supervisor {
	return &Supervisor{
		ConfigPath: configPath,
		LoadConfig: loader,
		Store:      store,
		Events:     broker,
		Runner:     system.ExecRunner{Timeout: 10 * time.Second},
		ProbeEvery: 15 * time.Second,
		byedpi:     newProc("byedpi"),
		tun2socks:  newProc("tun2socks"),
		reload:     make(chan struct{}, 1),
		probeNow:   make(chan struct{}, 1),
	}
}

// Reload signals that the sni_spoof config on disk has changed. Both loops
// are woken via separate channels (a single buffered channel races).
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
		slog.Error("snispoof supervisor missing LoadConfig")
		return
	}
	probeTicker := time.NewTicker(s.ProbeEvery)
	defer probeTicker.Stop()

	go s.manageChild(ctx)

	for {
		select {
		case <-ctx.Done():
			if cfg, err := s.LoadConfig(); err == nil {
				s.stopAll(cfg)
			}
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
	fail := func(cfg stack.SNISpoofConfig, msg string, err error, wait time.Duration) {
		s.log("error", msg, err)
		s.stopAll(cfg)
		s.publishStatus(cfg, false)
		s.waitReloadOrCancel(ctx, wait)
	}
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
			s.stopAll(cfg)
			s.publishStatus(cfg, false)
			s.waitReloadOrCancel(ctx, 5*time.Second)
			continue
		}
		byedpiBin, tun2socksBin, _, _, logPath, _ := EffectivePaths(cfg)
		if _, err := os.Stat(byedpiBin); err != nil {
			fail(cfg, "byedpi binary not found at "+byedpiBin, err, 30*time.Second)
			continue
		}
		if _, err := os.Stat(tun2socksBin); err != nil {
			fail(cfg, "tun2socks binary not found at "+tun2socksBin, err, 30*time.Second)
			continue
		}
		if err := EnsureConfigDir(cfg); err != nil {
			fail(cfg, "create config dir", err, 10*time.Second)
			continue
		}
		byeArgs, err := ByeDPIArgs(cfg)
		if err != nil {
			fail(cfg, "render byedpi args", err, 30*time.Second)
			continue
		}
		if err := s.byedpi.start(byedpiBin, byeArgs, logPath, s.log); err != nil {
			fail(cfg, "start byedpi", err, backoff)
			backoff = bumpBackoff(backoff)
			continue
		}
		if err := s.tun2socks.start(tun2socksBin, Tun2socksArgs(cfg, s.logLevel()), logPath, s.log); err != nil {
			fail(cfg, "start tun2socks", err, backoff)
			backoff = bumpBackoff(backoff)
			continue
		}
		if err := waitDevice(ctx, s.Runner, cfg.EffectiveTunName(), 8*time.Second); err != nil {
			fail(cfg, "wait for tun device", err, backoff)
			backoff = bumpBackoff(backoff)
			continue
		}
		if err := setupRoute(ctx, s.Runner, cfg); err != nil {
			fail(cfg, "program policy route", err, backoff)
			backoff = bumpBackoff(backoff)
			continue
		}
		s.setRouteReady(true)
		backoff = time.Second
		s.publishStatus(cfg, true)
		// Block until a child exits or a reload is requested, then tear the
		// whole thing down and let the loop bring it back up cleanly.
		s.waitChildrenOrReload(ctx)
		s.stopAll(cfg)
	}
}

func bumpBackoff(d time.Duration) time.Duration {
	if d < 60*time.Second {
		return d * 2
	}
	return d
}

// stopAll removes the route then stops both children. Route teardown uses a
// fresh context so it still runs during daemon shutdown.
func (s *Supervisor) stopAll(cfg stack.SNISpoofConfig) {
	if s.getRouteReady() {
		bg, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		teardownRoute(bg, s.Runner, cfg)
		cancel()
		s.setRouteReady(false)
	}
	s.tun2socks.stop(s.log)
	s.byedpi.stop(s.log)
}

// Restart forces a full teardown + respawn cycle.
func (s *Supervisor) Restart() {
	if cfg, err := s.LoadConfig(); err == nil {
		s.stopAll(cfg)
	}
	s.Reload()
}

func (s *Supervisor) waitChildrenOrReload(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		bye := s.byedpi.exitedCh()
		t2s := s.tun2socks.exitedCh()
		select {
		case <-ctx.Done():
			return
		case <-s.reload:
			return
		case <-bye:
			return
		case <-t2s:
			return
		case <-t.C:
			if !s.byedpi.alive() || !s.tun2socks.alive() {
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

func (s *Supervisor) logLevel() string {
	if s.LogLevel != nil {
		if lvl := s.LogLevel(); lvl != "" {
			return lvl
		}
	}
	return "warning"
}

func (s *Supervisor) setRouteReady(v bool) {
	s.mu.Lock()
	s.routeReady = v
	s.mu.Unlock()
}

func (s *Supervisor) getRouteReady() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.routeReady
}

func (s *Supervisor) baseStatus(cfg stack.SNISpoofConfig, managed bool) Status {
	byedpiBin, tun2socksBin, _, _, _, _ := EffectivePaths(cfg)
	_, byeErr := os.Stat(byedpiBin)
	_, t2sErr := os.Stat(tun2socksBin)
	return Status{
		Enabled:          cfg.Enabled,
		Managed:          managed,
		Listen:           cfg.EffectiveListen(),
		OutboundTag:      cfg.EffectiveOutboundTag(),
		Method:           cfg.EffectiveMethod(),
		FakeDomain:       cfg.EffectiveFakeDomain(),
		TunName:          cfg.EffectiveTunName(),
		ByeDPIBinary:     byedpiBin,
		Tun2socksBinary:  tun2socksBin,
		ByeDPIPID:        s.byedpi.pid(),
		Tun2socksPID:     s.tun2socks.pid(),
		ByeDPIRunning:    s.byedpi.alive(),
		Tun2socksRunning: s.tun2socks.alive(),
		RouteReady:       s.getRouteReady(),
		BinariesFound:    byeErr == nil && t2sErr == nil,
		EnabledSites:     len(cfg.EnabledSites()),
		TotalSites:       len(cfg.Sites),
		RestartCount:     s.byedpi.restartCount() + s.tun2socks.restartCount(),
		StartedAt:        s.byedpi.startedAtTime(),
	}
}

func (s *Supervisor) publishStatus(cfg stack.SNISpoofConfig, managed bool) {
	_, _, _, statePath, _, _ := EffectivePaths(cfg)
	st := s.baseStatus(cfg, managed)
	s.Store.ReplaceStatus(st)
	_ = s.Store.Persist(statePath)
	if s.Events != nil {
		s.Events.Publish("snispoof.status", map[string]any{
			"enabled": st.Enabled, "byedpi": st.ByeDPIRunning,
			"tun2socks": st.Tun2socksRunning, "route_ready": st.RouteReady,
		})
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
	_, _, _, statePath, _, _ := EffectivePaths(cfg)
	s.Store.UpdateStatus(func(st *Status) {
		base := s.baseStatus(cfg, st.Managed)
		base.ListenOK = result.ListenOK
		base.HealthOK = result.HealthOK
		base.LastProbe = time.Now()
		base.LatencyMS = result.LatencyMS
		if result.Error != "" {
			base.LastError = result.Error
		}
		*st = base
	})
	_ = s.Store.Persist(statePath)
	if !result.OK {
		s.log("warn", "probe failed", errors.New(result.Error))
	}
	if s.Events != nil {
		s.Events.Publish("snispoof.probe", map[string]any{
			"ok": result.OK, "listen_ok": result.ListenOK, "health_ok": result.HealthOK,
			"latency_ms": result.LatencyMS, "error": result.Error,
		})
	}
}

func (s *Supervisor) log(level, msg string, err error) {
	s.Store.AppendLog(level, msg, err)
	switch level {
	case "error":
		slog.Error("snispoof: "+msg, "err", err)
	case "warn":
		slog.Warn("snispoof: "+msg, "err", err)
	default:
		slog.Info("snispoof: " + msg)
	}
	if s.Events != nil {
		ev := map[string]any{"level": level, "msg": msg}
		if err != nil {
			ev["err"] = err.Error()
		}
		s.Events.Publish("snispoof.log", ev)
	}
}
