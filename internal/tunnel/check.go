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
	Probe       string `json:"probe,omitempty"`
	LatencyMS   int64  `json:"latency_ms,omitempty"`
	Error       string `json:"error,omitempty"`
}

func CheckAll(ctx context.Context, tunnels []stack.TunnelConfig, probes []stack.ProbeTarget) []Check {
	out := make([]Check, 0, len(tunnels))
	for _, t := range tunnels {
		out = append(out, CheckOne(ctx, t, probes))
	}
	return out
}

func CheckOne(ctx context.Context, t stack.TunnelConfig, probes []stack.ProbeTarget) Check {
	c := Check{Name: t.Name, Type: t.Type, Interface: t.Interface, SystemdUnit: t.SystemdUnit, Priority: t.Priority}
	ip, err := ifaceIPv4(ctx, t.Interface)
	if err != nil {
		c.Error = err.Error()
		return c
	}
	c.Up, c.IPv4 = true, ip
	if len(probes) == 0 {
		probes = []stack.ProbeTarget{{Address: "1.1.1.1", Port: 443}}
	}
	var errors []string
	for _, probe := range probes {
		if probe.Address == "" {
			continue
		}
		if probe.Port == 0 {
			probe.Port = 443
		}
		start := time.Now()
		d := net.Dialer{
			Timeout:   3 * time.Second,
			LocalAddr: &net.TCPAddr{IP: net.ParseIP(ip)},
			Control:   bindControl(t.Interface),
		}
		target := fmt.Sprintf("%s:%d", probe.Address, probe.Port)
		conn, err := d.DialContext(ctx, "tcp", target)
		latency := time.Since(start).Milliseconds()
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", target, err))
			c.LatencyMS = latency
			continue
		}
		_ = conn.Close()
		c.Healthy = true
		c.Probe = target
		c.LatencyMS = latency
		return c
	}
	c.Error = strings.Join(errors, "; ")
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
