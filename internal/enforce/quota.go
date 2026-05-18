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
	actions := []QuotaAction{}
	for _, u := range cfg.Xray.Users {
		if !u.Enabled {
			continue
		}
		// Check the lifetime quota first; if it's blown, that's the
		// most specific reason and we don't need to also flag the
		// period sub-cap.
		if u.QuotaBytes > 0 {
			total := pairTotal(state.Totals[u.Email])
			if total >= u.QuotaBytes {
				actions = append(actions, QuotaAction{
					Email:      u.Email,
					UsedBytes:  total,
					QuotaBytes: u.QuotaBytes,
					Action:     "disable-user",
					Reason:     fmt.Sprintf("used %d bytes >= total quota %d bytes", total, u.QuotaBytes),
				})
				continue
			}
		}
		// Period caps. The user remains enabled as long as every period
		// counter is under its cap; any single exceeded cap disables them
		// until that counter rolls over.
		periods := []struct {
			label string
			used  int64
			cap   int64
		}{
			{"daily", pairTotal(state.Daily[u.Email]), u.DailyQuotaBytes},
			{"weekly", pairTotal(state.Weekly[u.Email]), u.WeeklyQuotaBytes},
			{"monthly", pairTotal(state.Monthly[u.Email]), u.MonthlyQuotaBytes},
		}
		for _, p := range periods {
			if p.cap > 0 && p.used >= p.cap {
				actions = append(actions, QuotaAction{
					Email:      u.Email,
					UsedBytes:  p.used,
					QuotaBytes: p.cap,
					Action:     "disable-user",
					Reason:     fmt.Sprintf("%s used %d bytes >= %s quota %d bytes", p.label, p.used, p.label, p.cap),
				})
				break
			}
		}
	}
	return QuotaPlan{GeneratedAt: now.Unix(), Actions: actions}
}

func pairTotal(p usage.Pair) int64 { return p.Uplink + p.Downlink }

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
