package enforce

import (
	"testing"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/monitor"
	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
)

func TestPlanSessionLimitsKicksOldestIPs(t *testing.T) {
	cfg := stack.Config{Xray: stack.XrayConfig{Users: []stack.User{
		{Email: "amir", Enabled: true, MaxSessions: 2},
		{Email: "uncapped", Enabled: true}, // no limit
	}}}
	now := time.Now()
	snap := monitor.OnlineSnapshot{
		WindowSeconds: 300,
		Users: []monitor.OnlineUser{
			{
				Email: "amir",
				// Most-recent-first ordering, matching monitor.Online's output.
				IPDetails: []monitor.IPDetail{
					{IP: "10.0.0.1", LastSeen: now.Unix()},
					{IP: "10.0.0.2", LastSeen: now.Add(-1 * time.Minute).Unix()},
					{IP: "10.0.0.3", LastSeen: now.Add(-1 * time.Hour).Unix()},   // kick
					{IP: "10.0.0.4", LastSeen: now.Add(-2 * time.Hour).Unix()},   // kick
				},
			},
			{
				Email:     "uncapped",
				IPDetails: []monitor.IPDetail{{IP: "10.0.0.5"}, {IP: "10.0.0.6"}, {IP: "10.0.0.7"}},
			},
		},
	}
	plan := PlanSessionLimits(cfg, snap, now)
	if len(plan.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(plan.Actions))
	}
	a := plan.Actions[0]
	if a.Email != "amir" {
		t.Errorf("action email = %q, want amir", a.Email)
	}
	if a.MaxSessions != 2 {
		t.Errorf("action cap = %d, want 2", a.MaxSessions)
	}
	if len(a.KickIPs) != 2 || a.KickIPs[0] != "10.0.0.3" || a.KickIPs[1] != "10.0.0.4" {
		t.Errorf("kick ips = %v, want [10.0.0.3 10.0.0.4]", a.KickIPs)
	}
}

func TestPlanSessionLimitsUnderCapEmitsNothing(t *testing.T) {
	cfg := stack.Config{Xray: stack.XrayConfig{Users: []stack.User{
		{Email: "amir", Enabled: true, MaxSessions: 5},
	}}}
	snap := monitor.OnlineSnapshot{Users: []monitor.OnlineUser{{
		Email:     "amir",
		IPDetails: []monitor.IPDetail{{IP: "10.0.0.1"}, {IP: "10.0.0.2"}},
	}}}
	if len(PlanSessionLimits(cfg, snap, time.Now()).Actions) != 0 {
		t.Errorf("under-cap user should produce no actions")
	}
}
