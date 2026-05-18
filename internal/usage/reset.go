// Scheduled per-period counter resets.
//
// Each user keeps three counters (Daily, Weekly, Monthly) inside UserState.
// PeriodMeta stores the next reset deadline for each, in server-local time
// expressed as unix seconds. When a deadline passes, the matching counter
// is zeroed and the deadline is advanced to the next boundary.
//
// Boundaries (server local):
//   - Daily   → next occurrence of the user's daily_reset_hhmm (defaults 00:00).
//   - Weekly  → next Monday at 00:00.
//   - Monthly → 1st of next month at 00:00.
//
// The scheduler is cheap: it walks the user list once per minute and acts
// only when something is overdue. Saves to disk only when at least one
// counter actually rolled over.
package usage

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// PeriodUser is the slice of stack.User the reset loop needs. Keeps the
// usage package free of an import cycle on stack.
type PeriodUser struct {
	Email          string
	DailyResetHHMM string
}

// ResetConfig is the slice of stack config the period-reset loop needs.
type ResetConfig struct {
	Path  string
	Users []PeriodUser
}

// RunResetLoop checks every minute whether any per-user period counter is
// due to roll over and rewrites the on-disk state when it does.
func RunResetLoop(ctx context.Context, getCfg func() ResetConfig) {
	tick := time.NewTicker(60 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			runOneReset(getCfg(), time.Now())
		}
	}
}

func runOneReset(cfg ResetConfig, now time.Time) {
	if cfg.Path == "" {
		return
	}
	state, err := LoadUserState(cfg.Path)
	if err != nil {
		slog.Warn("usage reset: load state", "err", err)
		return
	}
	changed, next := ApplyPeriodResets(state, cfg.Users, now)
	if !changed {
		return
	}
	if err := SaveUserState(cfg.Path, next); err != nil {
		slog.Warn("usage reset: save state", "err", err)
	}
}

// ApplyPeriodResets walks every known user and rolls over any expired
// daily/weekly/monthly counter. Returns true if anything changed.
//
// For users whose meta has no deadline yet (first observation), this
// seeds the deadlines from `now` and does not zero the counter — the
// counter has only been accumulating since the panel started, so
// preserving it gives a more honest first-day reading.
func ApplyPeriodResets(st UserState, users []PeriodUser, now time.Time) (bool, UserState) {
	st = ensureUser(st)
	changed := false
	for _, u := range users {
		meta := st.Periods[u.Email]
		hh, mm := parseHHMMOrDefault(u.DailyResetHHMM)
		if meta.DailyResetAt == 0 {
			meta.DailyResetAt = nextDailyReset(now, hh, mm).Unix()
			changed = true
		} else if now.Unix() >= meta.DailyResetAt {
			st.Daily[u.Email] = Pair{}
			meta.DailyResetAt = nextDailyReset(now, hh, mm).Unix()
			changed = true
		}
		if meta.WeeklyResetAt == 0 {
			meta.WeeklyResetAt = nextWeeklyReset(now).Unix()
			changed = true
		} else if now.Unix() >= meta.WeeklyResetAt {
			st.Weekly[u.Email] = Pair{}
			meta.WeeklyResetAt = nextWeeklyReset(now).Unix()
			changed = true
		}
		if meta.MonthlyResetAt == 0 {
			meta.MonthlyResetAt = nextMonthlyReset(now).Unix()
			changed = true
		} else if now.Unix() >= meta.MonthlyResetAt {
			st.Monthly[u.Email] = Pair{}
			meta.MonthlyResetAt = nextMonthlyReset(now).Unix()
			changed = true
		}
		st.Periods[u.Email] = meta
	}
	if changed {
		st.UpdatedAt = now.Unix()
	}
	return changed, st
}

// nextDailyReset returns the next moment at HH:MM server-local after now.
// If today's HH:MM is still in the future, that's the next reset; otherwise
// tomorrow's HH:MM is returned.
func nextDailyReset(now time.Time, hh, mm int) time.Time {
	loc := now.Location()
	t := time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, loc)
	if !t.After(now) {
		t = t.Add(24 * time.Hour)
	}
	return t
}

// nextWeeklyReset returns the next Monday 00:00 server-local after now.
// time.Weekday() is Sunday=0; Monday=1, so daysUntilMonday is:
//
//	Sun(0) → 1, Mon(1) → 7, Tue(2) → 6, …, Sat(6) → 2.
func nextWeeklyReset(now time.Time) time.Time {
	loc := now.Location()
	wd := int(now.Weekday())
	days := (8 - wd) % 7
	if days == 0 {
		days = 7
	}
	t := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	return t.AddDate(0, 0, days)
}

// nextMonthlyReset returns the 1st of next month at 00:00 server-local.
func nextMonthlyReset(now time.Time) time.Time {
	loc := now.Location()
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	return first.AddDate(0, 1, 0)
}

func parseHHMMOrDefault(s string) (int, int) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 {
		return 0, 0
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return 0, 0
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return 0, 0
	}
	return h, m
}
