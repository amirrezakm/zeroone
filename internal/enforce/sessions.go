// Session-count enforcement. When a user has max_sessions > 0 and the
// online tracker reports more distinct client IPs than the cap, this
// kicks the oldest (least-recently-seen) IPs back down to max_sessions.
//
// The shape — kill oldest first — matches what an operator most often
// wants: a phone that hasn't sent a packet in 4 hours should yield to
// the laptop the user is actively using right now.
package enforce

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/monitor"
	"github.com/sakhtar/xray-stack-zeroone/internal/sessions"
	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
	"github.com/sakhtar/xray-stack-zeroone/internal/system"
)

type SessionAction struct {
	Email       string   `json:"email"`
	MaxSessions int      `json:"max_sessions"`
	SeenIPs     int      `json:"seen_ips"`
	KickIPs     []string `json:"kick_ips"`
	Reason      string   `json:"reason"`
}

type SessionPlan struct {
	GeneratedAt int64           `json:"generated_at"`
	WindowSecs  int             `json:"window_seconds"`
	Actions     []SessionAction `json:"actions"`
}

// PlanSessionLimits inspects the online snapshot and returns the list of
// (user, ips-to-kick) pairs implied by each user's MaxSessions setting.
// Users without a cap or under the cap produce no action.
func PlanSessionLimits(cfg stack.Config, snap monitor.OnlineSnapshot, now time.Time) SessionPlan {
	caps := map[string]int{}
	for _, u := range cfg.Xray.Users {
		if u.MaxSessions > 0 {
			caps[u.Email] = u.MaxSessions
		}
	}
	plan := SessionPlan{GeneratedAt: now.Unix(), WindowSecs: snap.WindowSeconds}
	for _, u := range snap.Users {
		cap, ok := caps[u.Email]
		if !ok {
			continue
		}
		// IPDetails is sorted most-recent-first by monitor.Online; the
		// excess at the tail is the oldest. Keep [0..cap), kick the rest.
		if len(u.IPDetails) <= cap {
			continue
		}
		kicked := make([]string, 0, len(u.IPDetails)-cap)
		for _, d := range u.IPDetails[cap:] {
			if d.IP != "" {
				kicked = append(kicked, d.IP)
			}
		}
		if len(kicked) == 0 {
			continue
		}
		plan.Actions = append(plan.Actions, SessionAction{
			Email:       u.Email,
			MaxSessions: cap,
			SeenIPs:     len(u.IPDetails),
			KickIPs:     kicked,
			Reason:      fmt.Sprintf("%d active IPs > max_sessions %d", len(u.IPDetails), cap),
		})
	}
	return plan
}

// ApplySessionPlan runs `ss -ntK` for each (port, ip) pair in the plan.
// Returns the total number of TCP sockets killed.
func ApplySessionPlan(ctx context.Context, runner system.Runner, ports []int, plan SessionPlan) (int, error) {
	if len(plan.Actions) == 0 || len(ports) == 0 {
		return 0, nil
	}
	killed := 0
	for _, action := range plan.Actions {
		res, err := sessions.KillByPeerIPs(ctx, runner, ports, action.KickIPs)
		if err != nil {
			return killed, err
		}
		killed += res.Killed
	}
	return killed, nil
}

// EnforceConfig is the slice of stack config the loop needs each tick.
type EnforceConfig struct {
	Cfg   stack.Config
	Ports []int
}

// RunSessionEnforcer ticks every 60s. On each tick it builds an online
// snapshot, plans the kicks implied by max_sessions, and runs them.
// No-op when no user has max_sessions > 0.
func RunSessionEnforcer(ctx context.Context, getCfg func() EnforceConfig, getSnapshot func(ctx context.Context) (monitor.OnlineSnapshot, error)) {
	tick := time.NewTicker(60 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			cfg := getCfg()
			any := false
			for _, u := range cfg.Cfg.Xray.Users {
				if u.MaxSessions > 0 {
					any = true
					break
				}
			}
			if !any {
				continue
			}
			snap, err := getSnapshot(ctx)
			if err != nil {
				slog.Debug("session enforcer: snapshot", "err", err)
				continue
			}
			plan := PlanSessionLimits(cfg.Cfg, snap, time.Now())
			if len(plan.Actions) == 0 {
				continue
			}
			killed, err := ApplySessionPlan(ctx, nil, cfg.Ports, plan)
			if err != nil {
				slog.Warn("session enforcer: apply", "err", err)
				continue
			}
			slog.Info("session enforcer kicked over-limit IPs", "actions", len(plan.Actions), "killed", killed)
		}
	}
}
