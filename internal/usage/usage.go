package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/system"
)

// SyncConfig is the slice of stack config the periodic syncer needs.
// Kept narrow to avoid an import cycle with the stack package.
type SyncConfig struct {
	Path       string
	APIAddress string
}

// Run periodically syncs per-user uplink/downlink totals from Xray's stats
// API into the on-disk state file. Without this loop the panel only updates
// totals when an admin clicks Sync, leaving quota enforcement on stale data.
// If observer is non-nil, each tick's raw counters are forwarded so the
// metrics collector can compute per-user bandwidth.
func Run(ctx context.Context, getCfg func() SyncConfig, observer func(map[string]Pair, time.Time)) {
	tick := time.NewTicker(60 * time.Second)
	defer tick.Stop()
	syncOnce(ctx, getCfg(), observer)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			syncOnce(ctx, getCfg(), observer)
		}
	}
}

func syncOnce(ctx context.Context, cfg SyncConfig, observer func(map[string]Pair, time.Time)) {
	if cfg.Path == "" || cfg.APIAddress == "" {
		return
	}
	raw, err := QueryXrayUsers(ctx, nil, cfg.APIAddress)
	if err != nil {
		slog.Debug("usage sync: query xray", "err", err)
		return
	}
	now := time.Now()
	if observer != nil {
		observer(raw, now)
	}
	state, err := LoadUserState(cfg.Path)
	if err != nil {
		slog.Warn("usage sync: load state", "err", err)
		return
	}
	state = SyncUsers(state, raw, now)
	if err := SaveUserState(cfg.Path, state); err != nil {
		slog.Warn("usage sync: save state", "err", err)
	}
}

type Pair struct {
	Uplink   int64 `json:"uplink"`
	Downlink int64 `json:"downlink"`
}
type SocksPair struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
}

type UserState struct {
	Totals    map[string]Pair       `json:"totals"`
	LastRaw   map[string]Pair       `json:"last_raw"`
	Daily     map[string]Pair       `json:"daily,omitempty"`
	Weekly    map[string]Pair       `json:"weekly,omitempty"`
	Monthly   map[string]Pair       `json:"monthly,omitempty"`
	Periods   map[string]PeriodMeta `json:"periods,omitempty"`
	UpdatedAt int64                 `json:"updated_at"`
}

// PeriodMeta tracks when each per-user period counter is next due to roll
// over. Stored as unix seconds in server-local time; recomputed whenever
// a counter is reset so the next deadline is always in the future.
type PeriodMeta struct {
	DailyResetAt   int64 `json:"daily_reset_at,omitempty"`
	WeeklyResetAt  int64 `json:"weekly_reset_at,omitempty"`
	MonthlyResetAt int64 `json:"monthly_reset_at,omitempty"`
}

type SocksState struct {
	Totals    map[string]SocksPair `json:"totals"`
	LastRaw   map[string]int64     `json:"last_raw"`
	UpdatedAt int64                `json:"updated_at"`
}

type UserView struct {
	Email    string `json:"email"`
	Uplink   int64  `json:"uplink"`
	Downlink int64  `json:"downlink"`
	Total    int64  `json:"total"`
}
type SocksView struct {
	User     string `json:"user"`
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
	Total    int64  `json:"total"`
}

func LoadUserState(path string) (UserState, error) {
	var st UserState
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ensureUser(st), nil
		}
		return st, err
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &st); err != nil {
			return st, err
		}
	}
	return ensureUser(st), nil
}
func ensureUser(st UserState) UserState {
	if st.Totals == nil {
		st.Totals = map[string]Pair{}
	}
	if st.LastRaw == nil {
		st.LastRaw = map[string]Pair{}
	}
	if st.Daily == nil {
		st.Daily = map[string]Pair{}
	}
	if st.Weekly == nil {
		st.Weekly = map[string]Pair{}
	}
	if st.Monthly == nil {
		st.Monthly = map[string]Pair{}
	}
	if st.Periods == nil {
		st.Periods = map[string]PeriodMeta{}
	}
	return st
}

func SaveUserState(path string, st UserState) error { return saveJSON(path, st) }

func SyncUsers(st UserState, raw map[string]Pair, now time.Time) UserState {
	st = ensureUser(st)
	for email, cur := range raw {
		last := st.LastRaw[email]
		du, dd := cur.Uplink-last.Uplink, cur.Downlink-last.Downlink
		// xray's counters reset to zero when a user is removed and re-added
		// or after `xray api stats --reset`. Treat negative deltas as a
		// fresh baseline (full counter is the contribution this tick).
		if du < 0 {
			du = cur.Uplink
		}
		if dd < 0 {
			dd = cur.Downlink
		}
		if du > 0 || dd > 0 {
			st.Totals[email] = addPair(st.Totals[email], du, dd)
			st.Daily[email] = addPair(st.Daily[email], du, dd)
			st.Weekly[email] = addPair(st.Weekly[email], du, dd)
			st.Monthly[email] = addPair(st.Monthly[email], du, dd)
		}
		st.LastRaw[email] = cur
	}
	st.UpdatedAt = now.Unix()
	return st
}

func addPair(p Pair, du, dd int64) Pair {
	if du > 0 {
		p.Uplink += du
	}
	if dd > 0 {
		p.Downlink += dd
	}
	return p
}

func ResetUsers(st UserState, raw map[string]Pair, known []string, now time.Time) UserState {
	st = ensureUser(st)
	st.Totals = map[string]Pair{}
	st.LastRaw = map[string]Pair{}
	st.Daily = map[string]Pair{}
	st.Weekly = map[string]Pair{}
	st.Monthly = map[string]Pair{}
	st.Periods = map[string]PeriodMeta{}
	for _, email := range known {
		st.Totals[email] = Pair{}
		st.LastRaw[email] = raw[email]
	}
	for email, value := range raw {
		st.Totals[email] = Pair{}
		st.LastRaw[email] = value
	}
	st.UpdatedAt = now.Unix()
	return st
}

func UserViews(st UserState) []UserView {
	st = ensureUser(st)
	out := make([]UserView, 0, len(st.Totals))
	for email, p := range st.Totals {
		out = append(out, UserView{Email: email, Uplink: p.Uplink, Downlink: p.Downlink, Total: p.Uplink + p.Downlink})
	}
	return out
}

func QueryXrayUsers(ctx context.Context, runner system.Runner, server string) (map[string]Pair, error) {
	if runner == nil {
		runner = system.ExecRunner{Timeout: 6 * time.Second}
	}
	res, err := runner.Run(ctx, "xray", "api", "statsquery", "--server="+server, "-pattern", "user>>>")
	if err != nil {
		return nil, fmt.Errorf("statsquery: %w: %s%s", err, res.Stdout, res.Stderr)
	}
	var payload struct {
		Stat []struct {
			Name  string `json:"name"`
			Value int64  `json:"value"`
		} `json:"stat"`
		Stats []struct {
			Name  string `json:"name"`
			Value int64  `json:"value"`
		} `json:"stats"`
	}
	if err := json.Unmarshal([]byte(res.Stdout), &payload); err != nil {
		return nil, err
	}
	out := map[string]Pair{}
	for _, s := range append(payload.Stat, payload.Stats...) {
		parts := splitStat(s.Name)
		if len(parts) < 4 || parts[0] != "user" || parts[1] == "router" {
			continue
		}
		p := out[parts[1]]
		switch parts[len(parts)-1] {
		case "uplink":
			p.Uplink += s.Value
		case "downlink":
			p.Downlink += s.Value
		}
		out[parts[1]] = p
	}
	return out, nil
}

func splitStat(s string) []string {
	return strings.Split(s, ">>>")
}

func saveJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
