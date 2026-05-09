package tunnel

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
)

type Check struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Interface   string `json:"interface"`
	SystemdUnit string `json:"systemd_unit"`
	Priority    int    `json:"priority"`
	Up          bool   `json:"up"`
	Healthy     bool   `json:"healthy"`
	IPv4        string `json:"ipv4,omitempty"`
	LatencyMS   int64  `json:"latency_ms,omitempty"`
	Error       string `json:"error,omitempty"`
}

func CheckAll(ctx context.Context, tunnels []stack.TunnelConfig, probeIP string, probePort int) []Check {
	out := make([]Check, 0, len(tunnels))
	for _, t := range tunnels {
		out = append(out, CheckOne(ctx, t, probeIP, probePort))
	}
	return out
}

func CheckOne(ctx context.Context, t stack.TunnelConfig, probeIP string, probePort int) Check {
	c := Check{Name: t.Name, Type: t.Type, Interface: t.Interface, SystemdUnit: t.SystemdUnit, Priority: t.Priority}
	ip, err := ifaceIPv4(ctx, t.Interface)
	if err != nil {
		c.Error = err.Error()
		return c
	}
	c.Up, c.IPv4 = true, ip
	start := time.Now()
	d := net.Dialer{Timeout: 3 * time.Second, LocalAddr: &net.TCPAddr{IP: net.ParseIP(ip)}}
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", probeIP, probePort))
	c.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		c.Error = err.Error()
		return c
	}
	_ = conn.Close()
	c.Healthy = true
	return c
}

func ifaceIPv4(ctx context.Context, name string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, "ip", "-4", "-o", "addr", "show", "dev", name, "scope", "global").Output()
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(b))
	if len(fields) < 4 {
		return "", fmt.Errorf("no IPv4 on %s", name)
	}
	return strings.Split(fields[3], "/")[0], nil
}
