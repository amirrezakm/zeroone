package tunnel

import (
	"context"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type ConnectRequest struct {
	Target    string `json:"target"`
	Port      int    `json:"port"`
	Route     string `json:"route"`
	Interface string `json:"interface,omitempty"`
	TimeoutMS int    `json:"timeout_ms,omitempty"`
}

type ConnectResult struct {
	OK        bool   `json:"ok"`
	Target    string `json:"target"`
	Address   string `json:"address"`
	Route     string `json:"route"`
	Interface string `json:"interface,omitempty"`
	LocalIPv4 string `json:"local_ipv4,omitempty"`
	LatencyMS int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

func TestConnect(ctx context.Context, req ConnectRequest) ConnectResult {
	target := normalizeTarget(req.Target)
	port := req.Port
	if port == 0 {
		port = 443
	}
	if port < 1 || port > 65535 {
		return ConnectResult{OK: false, Target: target, Route: req.Route, Error: "invalid port"}
	}
	if target == "" {
		return ConnectResult{OK: false, Route: req.Route, Error: "target is required"}
	}
	if strings.ContainsAny(target, "/\\ \t\r\n") {
		return ConnectResult{OK: false, Target: target, Route: req.Route, Error: "target must be a domain or IP address"}
	}

	timeout := time.Duration(req.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if timeout > 15*time.Second {
		timeout = 15 * time.Second
	}

	result := ConnectResult{Target: target, Route: routeName(req), Address: net.JoinHostPort(target, strconv.Itoa(port))}
	dialer := net.Dialer{Timeout: timeout}
	if req.Interface != "" {
		ip, err := ifaceIPv4(ctx, req.Interface)
		if err != nil {
			result.Interface = req.Interface
			result.Error = err.Error()
			return result
		}
		result.Interface = req.Interface
		result.LocalIPv4 = ip
		dialer.LocalAddr = &net.TCPAddr{IP: net.ParseIP(ip)}
		dialer.Control = bindControl(req.Interface)
	}

	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", result.Address)
	result.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	_ = conn.Close()
	result.OK = true
	return result
}

func normalizeTarget(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		if u, err := url.Parse(raw); err == nil {
			if host := u.Hostname(); host != "" {
				return host
			}
		}
	}
	host := raw
	if h, _, err := net.SplitHostPort(raw); err == nil {
		host = h
	}
	return strings.Trim(host, "[]")
}

func routeName(req ConnectRequest) string {
	if req.Route != "" {
		return req.Route
	}
	if req.Interface != "" {
		return req.Interface
	}
	return "direct"
}
