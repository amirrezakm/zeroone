package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/system"
)

type Pair struct {
	Uplink   int64 `json:"uplink"`
	Downlink int64 `json:"downlink"`
}
type SocksPair struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
}

type UserState struct {
	Totals    map[string]Pair `json:"totals"`
	LastRaw   map[string]Pair `json:"last_raw"`
	UpdatedAt int64           `json:"updated_at"`
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
	return st
}

func SaveUserState(path string, st UserState) error { return saveJSON(path, st) }

func SyncUsers(st UserState, raw map[string]Pair, now time.Time) UserState {
	st = ensureUser(st)
	for email, cur := range raw {
		last := st.LastRaw[email]
		total := st.Totals[email]
		du, dd := cur.Uplink-last.Uplink, cur.Downlink-last.Downlink
		if du < 0 {
			du = cur.Uplink
		}
		if dd < 0 {
			dd = cur.Downlink
		}
		if du > 0 {
			total.Uplink += du
		}
		if dd > 0 {
			total.Downlink += dd
		}
		st.Totals[email] = total
		st.LastRaw[email] = cur
	}
	st.UpdatedAt = now.Unix()
	return st
}

func ResetUsers(st UserState, raw map[string]Pair, known []string, now time.Time) UserState {
	st = ensureUser(st)
	st.Totals = map[string]Pair{}
	st.LastRaw = map[string]Pair{}
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
