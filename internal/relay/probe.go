package relay

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
)

// ProbeResult captures one health probe.
type ProbeResult struct {
	OK         bool   `json:"ok"`
	ListenOK   bool   `json:"listen_ok"`
	HealthOK   bool   `json:"health_ok"`
	LatencyMS  int64  `json:"latency_ms,omitempty"`
	Status     string `json:"status,omitempty"`
	Error      string `json:"error,omitempty"`
	Target     string `json:"target,omitempty"`
}

// Probe runs a TCP check on the listen port plus an HTTP CONNECT through
// the proxy to validate end-to-end relay functionality.
func Probe(ctx context.Context, c stack.RelayConfig, timeout time.Duration) ProbeResult {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	listen := c.EffectiveListen()
	_, _, _, _, healthProbe := EffectivePaths(c)
	out := ProbeResult{Target: healthProbe}

	tcpCtx, cancel := context.WithTimeout(ctx, timeout/2)
	defer cancel()
	d := net.Dialer{}
	conn, err := d.DialContext(tcpCtx, "tcp", listen)
	if err != nil {
		out.Error = fmt.Sprintf("tcp dial %s: %v", listen, err)
		return out
	}
	_ = conn.Close()
	out.ListenOK = true
	t0 := time.Now()
	if err := connectThroughProxy(ctx, listen, healthProbe, timeout); err != nil {
		out.Status = "proxy_connect_failed"
		out.Error = err.Error()
		return out
	}
	out.LatencyMS = time.Since(t0).Milliseconds()
	out.HealthOK = true
	out.OK = true
	out.Status = "ok"
	return out
}

// connectThroughProxy sends an HTTP/1.1 CONNECT to the local HTTP proxy and
// expects a 2xx response, which means mhrv-rs accepted and tunneled the
// session. We don't read further bytes — a successful tunnel handshake is
// enough to know the relay is alive.
func connectThroughProxy(ctx context.Context, proxyAddr, target string, timeout time.Duration) error {
	if !strings.Contains(target, ":") {
		target = target + ":443"
	}
	dctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	d := net.Dialer{}
	conn, err := d.DialContext(dctx, "tcp", proxyAddr)
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline, _ := dctx.Deadline()
	_ = conn.SetDeadline(deadline)
	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	if _, err := conn.Write([]byte(req)); err != nil {
		return err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: "CONNECT"})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("proxy returned %s", resp.Status)
	}
	return nil
}

// ParseProxyAddr accepts http://host:port or host:port and returns host:port.
func ParseProxyAddr(addr string) string {
	if strings.Contains(addr, "://") {
		if u, err := url.Parse(addr); err == nil && u.Host != "" {
			return u.Host
		}
	}
	return addr
}
