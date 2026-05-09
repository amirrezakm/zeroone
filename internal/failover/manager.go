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
	if decision.Pending {
		slog.Info("failover decision", "current", decision.Current, "desired", decision.Desired, "pending", decision.Pending, "confirmations", decision.ConfirmationCount, "reason", decision.Reason)
		return
	}
	if decision.Effective == decision.Current {
		slog.Debug("failover decision", "current", decision.Current, "desired", decision.Desired, "reason", decision.Reason)
		return
	}

	nextCfg := cfg
	ApplyMode(&nextCfg, decision.Effective)
	if err := stack.Save(m.ConfigPath, nextCfg); err != nil {
		slog.Error("failover save config", "err", err, "effective", decision.Effective)
		return
	}
	plan, err := m.Xray.Apply(ctx, nextCfg)
	if err != nil {
		slog.Error("failover apply xray", "err", err, "effective", decision.Effective)
		return
	}
	slog.Info("failover applied", "from", decision.Current, "to", decision.Effective, "changed", plan.Changed, "backup", plan.BackupPath)
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
