package enforce

import (
	"testing"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
	"github.com/sakhtar/xray-stack-zeroone/internal/usage"
)

func TestPlanQuotaDisablesOnlyEnabledOverQuotaUsers(t *testing.T) {
	cfg := stack.Config{Xray: stack.XrayConfig{Users: []stack.User{
		{Email: "over", UUID: "u1", Enabled: true, QuotaBytes: 100},
		{Email: "ok", UUID: "u2", Enabled: true, QuotaBytes: 1000},
		{Email: "disabled", UUID: "u3", Enabled: false, QuotaBytes: 1},
	}}}
	state := usage.SyncUsers(usage.UserState{}, map[string]usage.Pair{
		"over":     {Uplink: 50, Downlink: 50},
		"ok":       {Uplink: 10, Downlink: 20},
		"disabled": {Uplink: 999, Downlink: 999},
	}, time.Unix(1, 0))

	plan := PlanQuota(cfg, state, time.Unix(2, 0))
	if len(plan.Actions) != 1 || plan.Actions[0].Email != "over" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestApplyQuotaPlan(t *testing.T) {
	cfg := stack.Config{
		Server: stack.ServerConfig{AdminListen: "127.0.0.1:1", XrayConfigPath: "/tmp/xray.json"},
		Xray: stack.XrayConfig{
			Inbounds:  stack.InboundConfig{VLESSWSPort: 443},
			Outbounds: stack.OutboundSet{Proxy: stack.Outbound{Tag: "proxy"}},
			Users:     []stack.User{{Email: "over", UUID: "u1", Enabled: true}},
		},
	}
	err := ApplyQuotaPlan(&cfg, QuotaPlan{Actions: []QuotaAction{{Email: "over", Action: "disable-user"}}})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Xray.Users[0].Enabled {
		t.Fatal("user should be disabled")
	}
}
