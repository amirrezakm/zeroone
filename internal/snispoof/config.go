package snispoof

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/amirrezakm/zeroone/internal/stack"
)

// ByeDPIArgs renders the byedpi (ciadpi) command line for the configured
// desync method. byedpi is a local SOCKS5 proxy; tun2socks feeds it.
//
// Method presets (scoped to TLS via -K t so plain HTTP is untouched):
//
//	fake     -f 1+s -t <ttl> -n <fake_domain>   inject a fake ClientHello
//	                                             carrying the decoy SNI,
//	                                             low-TTL so only the DPI sees it
//	split    -s 1+s                              split ClientHello around the SNI
//	disorder -d 1+s                              split + reverse-order segments
//	auto     -A torst,ssl_err + the fake preset  retry desync after a reset
//
// Strategy, when set, replaces the method-derived desync flags verbatim
// (whitespace-split). ExtraArgs are always appended last. This keeps the
// common case turn-key while letting operators tune for their byedpi build.
func ByeDPIArgs(c stack.SNISpoofConfig) ([]string, error) {
	host, port, err := splitHostPort(c.EffectiveListen())
	if err != nil {
		return nil, err
	}
	args := []string{"-i", host, "-p", strconv.Itoa(port)}

	if strings.TrimSpace(c.Strategy) != "" {
		args = append(args, strings.Fields(c.Strategy)...)
		args = append(args, c.ExtraArgs...)
		return args, nil
	}

	ttl := strconv.Itoa(c.EffectiveFakeTTL())
	fake := []string{"-K", "t", "-f", "1+s", "-t", ttl, "-n", c.EffectiveFakeDomain()}
	switch c.EffectiveMethod() {
	case "fake":
		args = append(args, fake...)
	case "split":
		args = append(args, "-K", "t", "-s", "1+s")
	case "disorder":
		args = append(args, "-K", "t", "-d", "1+s")
	case "auto":
		args = append(args, "-A", "torst,ssl_err")
		args = append(args, fake...)
	default:
		return nil, fmt.Errorf("unknown method %q", c.EffectiveMethod())
	}
	args = append(args, c.ExtraArgs...)
	return args, nil
}

// Tun2socksArgs renders the tun2socks (xjasonlyu/tun2socks) command line.
// It creates/attaches the tun device and forwards everything it receives to
// byedpi's SOCKS5 listener. The kernel policy route (set up separately)
// decides which traffic reaches the device.
func Tun2socksArgs(c stack.SNISpoofConfig, logLevel string) []string {
	host, port, _ := splitHostPort(c.EffectiveListen())
	if host == "0.0.0.0" || host == "" {
		host = "127.0.0.1"
	}
	return []string{
		"-device", "tun://" + c.EffectiveTunName(),
		"-proxy", fmt.Sprintf("socks5://%s:%d", host, port),
		"-loglevel", tun2socksLogLevel(logLevel),
		"-mtu", strconv.Itoa(c.EffectiveMTU()),
	}
}

// tun2socksLogLevel maps a caller level (often xray's, e.g. "warning") to one
// tun2socks accepts: silent, error, warn, info, debug. tun2socks rejects
// "warning" outright, so we normalise rather than pass through.
func tun2socksLogLevel(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return "debug"
	case "info":
		return "info"
	case "warn", "warning":
		return "warn"
	case "error":
		return "error"
	case "none", "silent":
		return "silent"
	default:
		return "info"
	}
}

// EnsureConfigDir creates the plugin's state directory.
func EnsureConfigDir(c stack.SNISpoofConfig) error {
	_, _, dir, _, _, _ := EffectivePaths(c)
	return os.MkdirAll(dir, 0o755)
}

func splitHostPort(addr string) (string, int, error) {
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return "", 0, fmt.Errorf("invalid listen %q", addr)
	}
	host := addr[:i]
	portStr := addr[i+1:]
	if host == "" {
		host = "127.0.0.1"
	}
	if len(host) >= 2 && host[0] == '[' && host[len(host)-1] == ']' {
		host = host[1 : len(host)-1]
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port %q", portStr)
	}
	if port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("port out of range: %d", port)
	}
	return host, port, nil
}
