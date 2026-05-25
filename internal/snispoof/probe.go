package snispoof

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/amirrezakm/zeroone/internal/stack"
)

// ProbeResult captures one health probe.
type ProbeResult struct {
	OK        bool   `json:"ok"`
	ListenOK  bool   `json:"listen_ok"`
	HealthOK  bool   `json:"health_ok"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
	Status    string `json:"status,omitempty"`
	Error     string `json:"error,omitempty"`
	Target    string `json:"target,omitempty"`
}

// Probe checks the byedpi SOCKS5 listener is up and that a CONNECT through it
// to the health target succeeds (i.e. the desync proxy can reach upstream).
func Probe(ctx context.Context, c stack.SNISpoofConfig, timeout time.Duration) ProbeResult {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	listen := c.EffectiveListen()
	_, _, _, _, _, healthProbe := EffectivePaths(c)
	out := ProbeResult{Target: healthProbe}

	tcpCtx, cancel := context.WithTimeout(ctx, timeout/2)
	defer cancel()
	d := net.Dialer{}
	conn, err := d.DialContext(tcpCtx, "tcp", listen)
	if err != nil {
		out.Error = fmt.Sprintf("tcp dial %s: %v", listen, err)
		return out
	}
	out.ListenOK = true
	t0 := time.Now()
	if err := socks5Connect(ctx, conn, healthProbe, timeout); err != nil {
		_ = conn.Close()
		out.Status = "socks_connect_failed"
		out.Error = err.Error()
		return out
	}
	_ = conn.Close()
	out.LatencyMS = time.Since(t0).Milliseconds()
	out.HealthOK = true
	out.OK = true
	out.Status = "ok"
	return out
}

// socks5Connect performs a no-auth SOCKS5 handshake + CONNECT to target over
// an already-dialed proxy connection, returning nil when the proxy reports
// success. The target host is sent as a domain so byedpi resolves + desyncs.
func socks5Connect(ctx context.Context, conn net.Conn, target string, timeout time.Duration) error {
	host, port, err := parseHostPort(target)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = conn.SetDeadline(deadline)

	// greeting: VER=5, NMETHODS=1, METHOD=0 (no auth)
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return fmt.Errorf("socks greeting: %w", err)
	}
	resp := make([]byte, 2)
	if _, err := readFull(conn, resp); err != nil {
		return fmt.Errorf("socks greeting reply: %w", err)
	}
	if resp[0] != 0x05 || resp[1] != 0x00 {
		return fmt.Errorf("socks method rejected: %x", resp)
	}

	if len(host) > 255 {
		return fmt.Errorf("host too long")
	}
	req := []byte{0x05, 0x01, 0x00, 0x03, byte(len(host))}
	req = append(req, host...)
	var p [2]byte
	binary.BigEndian.PutUint16(p[:], uint16(port))
	req = append(req, p[0], p[1])
	if _, err := conn.Write(req); err != nil {
		return fmt.Errorf("socks connect: %w", err)
	}
	head := make([]byte, 4)
	if _, err := readFull(conn, head); err != nil {
		return fmt.Errorf("socks connect reply: %w", err)
	}
	if head[0] != 0x05 {
		return fmt.Errorf("bad socks version in reply: %x", head[0])
	}
	if head[1] != 0x00 {
		return fmt.Errorf("socks connect failed (rep=0x%02x)", head[1])
	}
	// Drain the bound address so the connection is left clean.
	var skip int
	switch head[3] {
	case 0x01:
		skip = 4 + 2
	case 0x04:
		skip = 16 + 2
	case 0x03:
		l := make([]byte, 1)
		if _, err := readFull(conn, l); err != nil {
			return fmt.Errorf("socks reply addr len: %w", err)
		}
		skip = int(l[0]) + 2
	default:
		return fmt.Errorf("unknown socks atyp: %x", head[3])
	}
	if skip > 0 {
		_, _ = readFull(conn, make([]byte, skip))
	}
	return nil
}

func readFull(conn net.Conn, b []byte) (int, error) {
	total := 0
	for total < len(b) {
		n, err := conn.Read(b[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func parseHostPort(target string) (string, int, error) {
	if !strings.Contains(target, ":") {
		return target, 443, nil
	}
	h, p, err := net.SplitHostPort(target)
	if err != nil {
		return "", 0, err
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port %q", p)
	}
	return h, n, nil
}
