package monitor

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/system"
)

type OnlineUser struct {
	Email              string     `json:"email"`
	Connections        int        `json:"connections"`         // new "accepted" flows seen in window
	ConnectionsPerMin  float64    `json:"connections_per_min"` // rate normalised to per-minute
	ActiveSessions     int        `json:"active_sessions"`     // concurrent established TCP sockets attributable to this user (heuristic via peer IP)
	LastSeen           int64      `json:"last_seen"`
	IPs                []string   `json:"ips"`
	IPDetails          []IPDetail `json:"ip_details"`
	RecentDestinations []string   `json:"recent_destinations"`
}

// IPDetail describes a single client IP seen for a user inside the window,
// with the timestamp of its most recent "accepted" line. The session-limit
// enforcer sorts on LastSeen to decide which IPs to kick when a user is
// over their max_sessions cap.
type IPDetail struct {
	IP       string `json:"ip"`
	LastSeen int64  `json:"last_seen"`
}

type OnlineSnapshot struct {
	WindowSeconds     int          `json:"window_seconds"`
	GeneratedAt       int64        `json:"generated_at"`
	Users             []OnlineUser `json:"users"`
	ActiveTCPSessions int          `json:"active_tcp_sessions"` // total concurrent established TCP across xray inbounds (from `ss`)
	TotalConnections  int          `json:"total_connections"`   // sum of connections seen across all users in the window
	ActiveByPort      map[int]int  `json:"active_by_port"`
	UniqueClientIPs   int          `json:"unique_client_ips"`
}

var emailRE = regexp.MustCompile(`email:\s*(\S+)`)
var fromRE = regexp.MustCompile(`from\s+(?:tcp:|udp:)?([^\s]+)\s+accepted\s+(tcp|udp):([^\s]+)\s+\[([^\]]+)\]`)

// Online aggregates panel-known users from xray's "accepted" log lines over
// the given window. Even at loglevel=warning, Xray emits accepted entries
// with an "email:" tag for VLESS clients, so this is a stable, low-cost
// signal that does not require a separate sidecar or stats schema.
func Online(ctx context.Context, runner system.Runner, windowSeconds int, ports []int) (OnlineSnapshot, error) {
	if windowSeconds <= 0 {
		windowSeconds = 300
	}
	if runner == nil {
		runner = system.ExecRunner{Timeout: 6 * time.Second}
	}
	out := OnlineSnapshot{
		WindowSeconds: windowSeconds,
		GeneratedAt:   time.Now().Unix(),
		Users:         []OnlineUser{},
		ActiveByPort:  map[int]int{},
	}

	res, err := runner.Run(ctx, "journalctl", "-u", "xray",
		"--since", fmt.Sprintf("%d seconds ago", windowSeconds),
		"--output=short-iso", "--no-pager")
	if err != nil {
		return out, err
	}

	type bucket struct {
		connections int
		lastSeen    time.Time
		ips         map[string]time.Time
		dests       []string
	}
	buckets := map[string]*bucket{}

	clientIPs := map[string]bool{}
	for _, line := range strings.Split(res.Stdout, "\n") {
		if !strings.Contains(line, " accepted ") {
			continue
		}
		em := emailRE.FindStringSubmatch(line)
		if len(em) != 2 {
			continue
		}
		email := em[1]
		ts, _ := parseLineTime(line)
		m := fromRE.FindStringSubmatch(line)
		clientIP := ""
		dest := ""
		if len(m) == 5 {
			raw := strings.TrimPrefix(m[1], "tcp:")
			raw = strings.TrimPrefix(raw, "udp:")
			if idx := strings.LastIndex(raw, ":"); idx > 0 {
				raw = raw[:idx]
			}
			clientIP = strings.Trim(raw, "[]")
			dest = m[3]
		}
		b := buckets[email]
		if b == nil {
			b = &bucket{ips: map[string]time.Time{}}
			buckets[email] = b
		}
		b.connections++
		if !ts.IsZero() && ts.After(b.lastSeen) {
			b.lastSeen = ts
		}
		if clientIP != "" {
			if prev, ok := b.ips[clientIP]; !ok || ts.After(prev) {
				b.ips[clientIP] = ts
			}
			clientIPs[clientIP] = true
		}
		if dest != "" && len(b.dests) < 8 {
			// keep up to 8 most-recent unique destinations per user
			seen := false
			for _, d := range b.dests {
				if d == dest {
					seen = true
					break
				}
			}
			if !seen {
				b.dests = append(b.dests, dest)
			}
		}
	}

	// Build a quick "peer IP → set of established remote ports" map from ss
	// so we can attribute active concurrent sessions to a user via their
	// recent client IPs. This is a heuristic — NAT'd IPs may collide across
	// users — but it's stable enough to give an honest active-now count.
	activeByIP := activeByPeerIP(ctx, ports)

	windowMinutes := float64(windowSeconds) / 60.0
	if windowMinutes < 1 {
		windowMinutes = 1
	}
	for email, b := range buckets {
		details := make([]IPDetail, 0, len(b.ips))
		for ip, ts := range b.ips {
			var lastSeen int64
			if !ts.IsZero() {
				lastSeen = ts.Unix()
			}
			details = append(details, IPDetail{IP: ip, LastSeen: lastSeen})
		}
		// Sort details by most-recent-first so the panel can show a stable
		// order and the session enforcer can pick the oldest tail to kick.
		sort.Slice(details, func(i, j int) bool {
			if details[i].LastSeen == details[j].LastSeen {
				return details[i].IP < details[j].IP
			}
			return details[i].LastSeen > details[j].LastSeen
		})
		ips := make([]string, 0, len(details))
		for _, d := range details {
			ips = append(ips, d.IP)
		}
		var lastSeen int64
		if !b.lastSeen.IsZero() {
			lastSeen = b.lastSeen.Unix()
		}
		active := 0
		for _, ip := range ips {
			active += activeByIP[ip]
		}
		out.Users = append(out.Users, OnlineUser{
			Email:              email,
			Connections:        b.connections,
			ConnectionsPerMin:  float64(b.connections) / windowMinutes,
			ActiveSessions:     active,
			LastSeen:           lastSeen,
			IPs:                ips,
			IPDetails:          details,
			RecentDestinations: b.dests,
		})
		out.TotalConnections += b.connections
	}
	sort.Slice(out.Users, func(i, j int) bool {
		if out.Users[i].LastSeen == out.Users[j].LastSeen {
			return out.Users[i].Connections > out.Users[j].Connections
		}
		return out.Users[i].LastSeen > out.Users[j].LastSeen
	})
	out.UniqueClientIPs = len(clientIPs)

	// Total active TCP across xray inbounds (server-wide concurrent count).
	for _, port := range ports {
		count := countEstablished(ctx, port)
		out.ActiveByPort[port] = count
		out.ActiveTCPSessions += count
	}
	return out, nil
}

// activeByPeerIP returns a map of peer-IP → concurrent-established-count
// across all xray-listening ports. Used to attribute "active sessions"
// to a panel-known user when their client IP shows up in the table.
func activeByPeerIP(ctx context.Context, ports []int) map[string]int {
	out := map[string]int{}
	for _, port := range ports {
		cmd := exec.CommandContext(ctx, "ss", "-ntH", "state", "established", fmt.Sprintf("( sport = :%d )", port))
		raw, err := cmd.Output()
		if err != nil {
			continue
		}
		// ss -nH columns: Recv-Q Send-Q LocalAddress:Port PeerAddress:Port
		scanner := bufio.NewScanner(strings.NewReader(string(raw)))
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 4 {
				continue
			}
			peer := fields[3]
			// Strip the IPv4-mapped-IPv6 prefix, then drop the trailing :port,
			// then strip any remaining brackets. ss formats peers as
			// "[::ffff:5.116.47.93]:31624" or "1.2.3.4:567" or "[2001:db8::1]:443".
			peer = strings.TrimPrefix(peer, "[::ffff:")
			if idx := strings.LastIndex(peer, ":"); idx > 0 {
				peer = peer[:idx]
			}
			peer = strings.Trim(peer, "[]")
			if peer == "" {
				continue
			}
			out[peer]++
		}
	}
	return out
}

func parseLineTime(line string) (time.Time, error) {
	// short-iso prefix: "2026-05-10T06:32:24+0330"
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return time.Time{}, nil
	}
	candidate := fields[0]
	for _, layout := range []string{"2006-01-02T15:04:05-0700", "2006-01-02T15:04:05Z07:00"} {
		if t, err := time.Parse(layout, candidate); err == nil {
			return t, nil
		}
	}
	return time.Time{}, nil
}

func countEstablished(ctx context.Context, port int) int {
	cmd := exec.CommandContext(ctx, "ss", "-ntH", "state", "established", fmt.Sprintf("( sport = :%d )", port))
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	count := 0
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			count++
		}
	}
	return count
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
