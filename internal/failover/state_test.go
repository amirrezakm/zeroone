package failover

import (
	"testing"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
	"github.com/sakhtar/xray-stack-zeroone/internal/tunnel"
)

func testConfig() stack.Config {
	return stack.Config{Xray: stack.XrayConfig{Outbounds: stack.OutboundSet{Proxy: stack.Outbound{Tag: "proxy", Interface: "tun0"}}}, Tunnels: []stack.TunnelConfig{{Interface: "tun0"}, {Interface: "tun1"}}, Failover: stack.FailoverConfig{FallbackOutboundTag: "priority-proxy", Confirmations: 2, CooldownSeconds: 60}}
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
