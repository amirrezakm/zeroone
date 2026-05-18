package failover

import (
	"context"
	"log/slog"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
	"github.com/sakhtar/xray-stack-zeroone/internal/tunnel"
	"github.com/sakhtar/xray-stack-zeroone/internal/xray"
)

type Manager struct {
	ConfigPath string
	State      State
	Xray       xray.Manager

	stateLoaded bool
	statePath   string
}

func (m *Manager) Run(ctx context.Context) {
	interval := 15 * time.Second
	for {
		cfg, err := stack.Load(m.ConfigPath)
		if err != nil {
			slog.Error("failover load config", "err", err)
		} else if cfg.Failover.Enabled {
			if cfg.Failover.IntervalSeconds > 0 {
				interval = time.Duration(cfg.Failover.IntervalSeconds) * time.Second
			}
			m.loadState(cfg.Server.FailoverStatePath)
			m.runOnce(ctx, *cfg)
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (m *Manager) runOnce(ctx context.Context, cfg stack.Config) {
	checks := tunnel.CheckAll(ctx, cfg.Tunnels, cfg.Failover.ProbeTargets())
	decision, nextState := Decide(cfg, m.State, checks, time.Now())
	m.setState(cfg.Server.FailoverStatePath, nextState)
	// Always emit a heartbeat at INFO so operators can grep for "failover tick"
	// and confirm the loop is alive. Compact summary: which tunnels are healthy
	// and what we're going to do.
	healthSummary := make([]string, 0, len(checks))
	for _, c := range checks {
		state := "down"
		if c.Healthy {
			state = "up"
		}
		healthSummary = append(healthSummary, c.Interface+":"+state)
	}
	slog.Info("failover tick", "mode", cfg.Failover.EffectiveMode(), "preferred", cfg.Failover.PreferredTunnel, "current", decision.Current.Interface, "desired", decision.Desired.Interface, "reason", decision.Reason, "pending", decision.Pending, "tunnels", healthSummary)
	if decision.Pending {
		return
	}
	if decision.Effective == decision.Current {
		return
	}

	nextCfg := cfg
	ApplyMode(&nextCfg, decision.Effective)
	if err := stack.Save(m.ConfigPath, nextCfg); err != nil {
		slog.Error("failover save config", "err", err, "effective", decision.Effective)
		m.recordTransition(cfg, decision, err)
		return
	}
	plan, err := m.Xray.Apply(ctx, nextCfg)
	if err != nil {
		slog.Error("failover apply xray", "err", err, "effective", decision.Effective)
		m.recordTransition(cfg, decision, err)
		return
	}
	slog.Info("failover applied", "from", decision.Current, "to", decision.Effective, "changed", plan.Changed, "backup", plan.BackupPath)
	m.recordTransition(cfg, decision, nil)
}

func (m *Manager) recordTransition(cfg stack.Config, d Decision, applyErr error) {
	path := cfg.Server.FailoverHistoryPath
	if path == "" {
		return
	}
	entry := Entry{T: time.Now().Unix(), From: d.Current, To: d.Effective, Reason: d.Reason}
	if applyErr != nil {
		entry.Error = applyErr.Error()
	}
	if err := AppendHistory(path, entry, DefaultHistoryRetention); err != nil {
		slog.Warn("failover history append failed", "path", path, "err", err)
	}
}

func (m *Manager) loadState(path string) {
	if path == "" {
		path = DefaultStatePath
	}
	if m.stateLoaded && m.statePath == path {
		return
	}
	st, err := LoadState(path)
	if err != nil {
		slog.Warn("failover state load failed", "path", path, "err", err)
	} else {
		m.State = st
	}
	m.statePath = path
	m.stateLoaded = true
}

func (m *Manager) setState(path string, st State) {
	if path == "" {
		path = DefaultStatePath
	}
	m.State = st
	if err := SaveState(path, st); err != nil {
		slog.Warn("failover state save failed", "path", path, "err", err)
	}
}
