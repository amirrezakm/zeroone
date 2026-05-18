package monitor

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
	"github.com/sakhtar/xray-stack-zeroone/internal/system"
)

type System struct {
	CPU     CPU             `json:"cpu"`
	RAM     RAM             `json:"ram"`
	Tunnels []InterfaceStat `json:"tunnels"`
	Updated int64           `json:"updated_at"`
}
type CPU struct {
	Percent float64 `json:"percent"`
	Detail  string  `json:"detail"`
}
type RAM struct {
	Percent    float64 `json:"percent"`
	UsedBytes  int64   `json:"used_bytes"`
	TotalBytes int64   `json:"total_bytes"`
	Detail     string  `json:"detail"`
}
type InterfaceStat struct {
	Name      string `json:"name"`
	RXBytes   int64  `json:"rx_bytes"`
	TXBytes   int64  `json:"tx_bytes"`
	RXDropped int64  `json:"rx_dropped"`
	TXDropped int64  `json:"tx_dropped"`
	RXErrors  int64  `json:"rx_errors"`
	TXErrors  int64  `json:"tx_errors"`
}
type Activity struct {
	Time        string `json:"time"`
	ClientIP    string `json:"client_ip"`
	Protocol    string `json:"protocol"`
	Destination string `json:"destination"`
	Outbound    string `json:"outbound"`
}

func SystemSnapshot(cfg stack.Config) System {
	out := System{CPU: cpu(), RAM: ram(), Updated: time.Now().Unix()}
	for _, t := range cfg.Tunnels {
		out.Tunnels = append(out.Tunnels, ifaceStat(t.Interface))
	}
	return out
}

func RecentActivity(ctx context.Context, runner system.Runner, email string, seconds, limit int) ([]Activity, error) {
	if runner == nil {
		runner = system.ExecRunner{Timeout: 6 * time.Second}
	}
	if seconds <= 0 {
		seconds = 120
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	res, err := runner.Run(ctx, "journalctl", "-u", "xray", "--since", fmt.Sprintf("%d seconds ago", seconds), "--output=short-iso", "--no-pager")
	if err != nil {
		return nil, err
	}
	lineRE := regexp.MustCompile(`^(\S+).*? from (?:tcp:)?([^ ]+) accepted (tcp|udp):([^ ]+) \[([^\]]+)\]`)
	items := []Activity{}
	seen := map[string]bool{}
	lines := strings.Split(res.Stdout, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if !strings.Contains(line, " accepted ") || !lineHasEmail(line, email) {
			continue
		}
		m := lineRE.FindStringSubmatch(line)
		if len(m) != 6 {
			continue
		}
		key := m[4] + "|" + m[5]
		if seen[key] {
			continue
		}
		seen[key] = true
		client := strings.TrimPrefix(m[2], "tcp:")
		if idx := strings.LastIndex(client, ":"); idx > 0 {
			client = client[:idx]
		}
		items = append(items, Activity{Time: cleanTime(m[1]), ClientIP: strings.Trim(client, "[]"), Protocol: m[3], Destination: m[4], Outbound: m[5]})
		if len(items) >= limit {
			break
		}
	}
	return items, nil
}

func lineHasEmail(line, email string) bool {
	for _, token := range strings.Fields(line) {
		if token == email {
			return strings.Contains(line, "email: "+email) || strings.Contains(line, "email:\t"+email)
		}
	}
	return false
}

func cpu() CPU {
	load, _ := os.ReadFile("/proc/loadavg")
	cores := 1
	if b, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		cores = max(1, strings.Count(string(b), "processor\t:"))
	}
	fields := strings.Fields(string(load))
	if len(fields) == 0 {
		return CPU{Detail: "unavailable"}
	}
	load1, _ := strconv.ParseFloat(fields[0], 64)
	percent := load1 / float64(cores) * 100
	if percent > 100 {
		percent = 100
	}
	return CPU{Percent: round(percent), Detail: fmt.Sprintf("load %s on %d core", fields[0], cores)}
}

func ram() RAM {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return RAM{Detail: "unavailable"}
	}
	defer f.Close()
	values := map[string]int64{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		parts := strings.Fields(strings.TrimSuffix(scanner.Text(), ":"))
		if len(parts) >= 2 {
			v, _ := strconv.ParseInt(parts[1], 10, 64)
			values[strings.TrimSuffix(parts[0], ":")] = v * 1024
		}
	}
	total := values["MemTotal"]
	available := values["MemAvailable"]
	used := total - available
	percent := 0.0
	if total > 0 {
		percent = float64(used) / float64(total) * 100
	}
	return RAM{Percent: round(percent), UsedBytes: used, TotalBytes: total, Detail: fmt.Sprintf("%s used of %s", HumanBytes(used), HumanBytes(total))}
}

func ifaceStat(name string) InterfaceStat {
	read := func(field string) int64 {
		b, err := os.ReadFile("/sys/class/net/" + name + "/statistics/" + field)
		return parseInt(b, err)
	}
	return InterfaceStat{
		Name:      name,
		RXBytes:   read("rx_bytes"),
		TXBytes:   read("tx_bytes"),
		RXDropped: read("rx_dropped"),
		TXDropped: read("tx_dropped"),
		RXErrors:  read("rx_errors"),
		TXErrors:  read("tx_errors"),
	}
}

func parseInt(b []byte, err error) int64 {
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	return v
}

func cleanTime(v string) string {
	if strings.Contains(v, "T") {
		parts := strings.SplitN(v, "T", 2)
		return parts[0] + " " + strings.Split(strings.Split(parts[1], "+")[0], ".")[0]
	}
	return v
}

func round(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}

func HumanBytes(value int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	n := float64(value)
	i := 0
	for n >= 1024 && i < len(units)-1 {
		n /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d %s", value, units[i])
	}
	return fmt.Sprintf("%.1f %s", n, units[i])
}
