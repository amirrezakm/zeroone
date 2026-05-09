package tunnel

import (
	"context"
	"log/slog"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
	"github.com/sakhtar/xray-stack-zeroone/internal/system"
)

type Supervisor struct {
	ConfigPath string
	Runner     system.Runner
	Cooldown   time.Duration
	Interval   time.Duration

	lastRestart map[string]time.Time
}

func (s *Supervisor) Run(ctx context.Context) {
	if s.Interval <= 0 {
		s.Interval = 30 * time.Second
	}
	if s.Cooldown <= 0 {
		s.Cooldown = 180 * time.Second
	}
	if s.lastRestart == nil {
		s.lastRestart = map[string]time.Time{}
	}
	for {
		s.runOnce(ctx)
		timer := time.NewTimer(s.Interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *Supervisor) runOnce(ctx context.Context) {
	cfg, err := stack.Load(s.ConfigPath)
	if err != nil {
		slog.Error("tunnel supervisor load config", "err", err)
		return
	}
	for _, t := range cfg.Tunnels {
		s.checkTunnel(ctx, t)
	}
	if err := s.isActive(ctx, "xray.service"); err != nil {
		slog.Warn("xray service is not active; systemd restart policy owns recovery", "err", err)
	}
}

func (s *Supervisor) checkTunnel(ctx context.Context, t stack.TunnelConfig) {
	if t.SystemdUnit == "" || t.Interface == "" {
		return
	}
	if err := s.isActive(ctx, t.SystemdUnit+".service"); err != nil {
		slog.Warn("tunnel service inactive", "unit", t.SystemdUnit, "err", err)
		s.restart(ctx, t, "service inactive")
		return
	}
	if _, err := ifaceIPv4(ctx, t.Interface); err != nil {
		slog.Warn("tunnel interface missing IPv4", "unit", t.SystemdUnit, "interface", t.Interface, "err", err)
		s.restart(ctx, t, "missing IPv4")
	}
}

func (s *Supervisor) restart(ctx context.Context, t stack.TunnelConfig, reason string) {
	now := time.Now()
	if last := s.lastRestart[t.SystemdUnit]; !last.IsZero() && now.Sub(last) < s.Cooldown {
		slog.Info("tunnel restart suppressed by cooldown", "unit", t.SystemdUnit, "reason", reason)
		return
	}
	runner := s.runner()
	if _, err := runner.Run(ctx, "systemctl", "restart", t.SystemdUnit+".service"); err != nil {
		slog.Error("tunnel restart failed", "unit", t.SystemdUnit, "reason", reason, "err", err)
		return
	}
	s.lastRestart[t.SystemdUnit] = now
	slog.Warn("tunnel restarted", "unit", t.SystemdUnit, "interface", t.Interface, "reason", reason)
}

func (s *Supervisor) isActive(ctx context.Context, unit string) error {
	_, err := s.runner().Run(ctx, "systemctl", "is-active", "--quiet", unit)
	return err
}

func (s *Supervisor) runner() system.Runner {
	if s.Runner != nil {
		return s.Runner
	}
	return system.ExecRunner{Timeout: 20 * time.Second}
}
