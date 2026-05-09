package enforce

import (
	"fmt"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
	"github.com/sakhtar/xray-stack-zeroone/internal/usage"
)

type QuotaAction struct {
	Email      string `json:"email"`
	UsedBytes  int64  `json:"used_bytes"`
	QuotaBytes int64  `json:"quota_bytes"`
	Action     string `json:"action"`
	Reason     string `json:"reason"`
}

type QuotaPlan struct {
	GeneratedAt int64         `json:"generated_at"`
	Actions     []QuotaAction `json:"actions"`
}

func PlanQuota(cfg stack.Config, state usage.UserState, now time.Time) QuotaPlan {
	totals := map[string]usage.Pair{}
	for _, v := range usage.UserViews(state) {
		totals[v.Email] = usage.Pair{Uplink: v.Uplink, Downlink: v.Downlink}
	}
	actions := []QuotaAction{}
	for _, u := range cfg.Xray.Users {
		if !u.Enabled || u.QuotaBytes <= 0 {
			continue
		}
		used := totals[u.Email]
		total := used.Uplink + used.Downlink
		if total >= u.QuotaBytes {
			actions = append(actions, QuotaAction{
				Email:      u.Email,
				UsedBytes:  total,
				QuotaBytes: u.QuotaBytes,
				Action:     "disable-user",
				Reason:     fmt.Sprintf("used %d bytes >= quota %d bytes", total, u.QuotaBytes),
			})
		}
	}
	return QuotaPlan{GeneratedAt: now.Unix(), Actions: actions}
}

func ApplyQuotaPlan(cfg *stack.Config, plan QuotaPlan) error {
	for _, action := range plan.Actions {
		if action.Action != "disable-user" {
			continue
		}
		if err := cfg.SetUserEnabled(action.Email, false); err != nil {
			return err
		}
	}
	return nil
}
