package failover

import (
	"testing"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
	"github.com/sakhtar/xray-stack-zeroone/internal/tunnel"
)

func testConfig() stack.Config {
	return stack.Config{Xray: stack.XrayConfig{Outbounds: stack.OutboundSet{Proxy: stack.Outbound{Tag: "proxy", Interface: "tun0"}}}, Tunnels: []stack.TunnelConfig{{Name: "company", Interface: "tun0"}, {Name: "backup", Interface: "tun1"}}, Failover: stack.FailoverConfig{FallbackOutboundTag: "priority-proxy", Confirmations: 2, CooldownSeconds: 60}}
}

func TestDecideRequiresConfirmations(t *testing.T) {
	cfg := testConfig()
	checks := []tunnel.Check{{Interface: "tun1", Healthy: true}}
	d, st := Decide(cfg, State{}, checks, time.Unix(100, 0))
	if !d.Pending || d.Effective.Interface != "tun0" || st.Count != 1 {
		t.Fatalf("first decision mismatch: %+v %+v", d, st)
	}
	d, st = Decide(cfg, st, checks, time.Unix(101, 0))
	if d.Pending || d.Effective.Interface != "tun1" || st.Count != 0 {
		t.Fatalf("second decision mismatch: %+v %+v", d, st)
	}
}

func TestDecideFallback(t *testing.T) {
	cfg := testConfig()
	cfg.Failover.Confirmations = 1
	d, _ := Decide(cfg, State{}, nil, time.Unix(100, 0))
	if d.Effective.OutboundTag != "priority-proxy" {
		t.Fatalf("expected fallback: %+v", d)
	}
}

func TestCurrentModeUsesAIOutboundOverride(t *testing.T) {
	cfg := testConfig()
	cfg.Xray.Routing.AIOutboundTag = "priority-proxy"
	mode := CurrentMode(cfg)
	if mode.OutboundTag != "priority-proxy" || mode.Interface != "" {
		t.Fatalf("unexpected mode: %+v", mode)
	}
}

func TestApplyModeClearsOverrideForProxy(t *testing.T) {
	cfg := testConfig()
	cfg.Xray.Routing.AIOutboundTag = "priority-proxy"
	ApplyMode(&cfg, Mode{OutboundTag: "proxy", Interface: "tun1"})
	if cfg.Xray.Routing.AIOutboundTag != "" || cfg.Xray.Outbounds.Proxy.Interface != "tun1" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestDesiredModePreferredHealthy(t *testing.T) {
	cfg := testConfig()
	cfg.Failover.Mode = stack.FailoverModePreferred
	cfg.Failover.PreferredTunnel = "backup"
	checks := []tunnel.Check{{Interface: "tun0", Healthy: true}, {Interface: "tun1", Healthy: true}}
	got := DesiredMode(cfg, checks)
	if got.Interface != "tun1" {
		t.Fatalf("preferred should pick tun1 when healthy, got %+v", got)
	}
}

func TestDesiredModePreferredFallsOver(t *testing.T) {
	cfg := testConfig()
	cfg.Failover.Mode = stack.FailoverModePreferred
	cfg.Failover.PreferredTunnel = "backup"
	checks := []tunnel.Check{{Interface: "tun0", Healthy: true}, {Interface: "tun1", Healthy: false}}
	got := DesiredMode(cfg, checks)
	if got.Interface != "tun0" {
		t.Fatalf("preferred should fall over to tun0 when tun1 unhealthy, got %+v", got)
	}
}

func TestDesiredModeManualSticksWhenHealthy(t *testing.T) {
	cfg := testConfig()
	cfg.Failover.Mode = stack.FailoverModeManual
	cfg.Failover.PreferredTunnel = "company"
	cfg.Xray.Outbounds.Proxy.Interface = "tun1" // current is tun1 (e.g. user previously switched)
	checks := []tunnel.Check{{Interface: "tun0", Healthy: true}, {Interface: "tun1", Healthy: true}}
	got := DesiredMode(cfg, checks)
	if got.Interface != "tun1" {
		t.Fatalf("manual should stay on current healthy tun1, got %+v", got)
	}
}

func TestDesiredModeManualFallsOverWhenDead(t *testing.T) {
	cfg := testConfig()
	cfg.Failover.Mode = stack.FailoverModeManual
	cfg.Failover.PreferredTunnel = "company"
	cfg.Xray.Outbounds.Proxy.Interface = "tun1"
	checks := []tunnel.Check{{Interface: "tun0", Healthy: true}, {Interface: "tun1", Healthy: false}}
	got := DesiredMode(cfg, checks)
	if got.Interface != "tun0" {
		t.Fatalf("manual should fall over to tun0 when current tun1 dies, got %+v", got)
	}
}

func TestDecidePreferredAutoReturnSkipsCooldown(t *testing.T) {
	cfg := testConfig()
	cfg.Failover.Mode = stack.FailoverModePreferred
	cfg.Failover.PreferredTunnel = "backup"
	cfg.Failover.Confirmations = 1
	cfg.Failover.CooldownSeconds = 300
	// Currently on tun0 (fallback), preferred is backup/tun1, both healthy → want to switch back
	cfg.Xray.Outbounds.Proxy.Interface = "tun0"
	checks := []tunnel.Check{{Interface: "tun0", Healthy: true}, {Interface: "tun1", Healthy: true}}
	// State says we changed 10s ago — well inside the 300s cooldown
	state := State{LastChangeUnix: 100}
	d, _ := Decide(cfg, state, checks, time.Unix(110, 0))
	if d.Pending || d.Effective.Interface != "tun1" {
		t.Fatalf("expected immediate switch back to preferred tun1, got pending=%v effective=%+v reason=%q", d.Pending, d.Effective, d.Reason)
	}
}

func TestDecidePreferredFallOverStillRespectsCooldown(t *testing.T) {
	cfg := testConfig()
	cfg.Failover.Mode = stack.FailoverModePreferred
	cfg.Failover.PreferredTunnel = "backup"
	cfg.Failover.Confirmations = 1
	cfg.Failover.CooldownSeconds = 300
	// Currently on tun1 (preferred, healthy), but tun1 dies → must fall over to tun0
	// Cooldown should STILL apply because we're NOT returning to preferred.
	cfg.Xray.Outbounds.Proxy.Interface = "tun1"
	checks := []tunnel.Check{{Interface: "tun0", Healthy: true}, {Interface: "tun1", Healthy: false}}
	state := State{LastChangeUnix: 100}
	d, _ := Decide(cfg, state, checks, time.Unix(110, 0))
	if !d.Pending || d.Reason != "cooldown active" {
		t.Fatalf("expected cooldown to block fall-over (not a preferred-return), got pending=%v reason=%q", d.Pending, d.Reason)
	}
}

func TestDesiredModeAutoUnchanged(t *testing.T) {
	cfg := testConfig() // default mode is auto
	checks := []tunnel.Check{{Interface: "tun0", Healthy: true}, {Interface: "tun1", Healthy: true}}
	got := DesiredMode(cfg, checks)
	if got.Interface != "tun0" {
		t.Fatalf("auto should pick first-priority healthy tun0, got %+v", got)
	}
}
