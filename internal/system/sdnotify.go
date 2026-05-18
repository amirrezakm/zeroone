package system

import (
	"context"
	"net"
	"os"
	"strconv"
	"time"
)

// Captured at package init so children spawned after we Unsetenv NOTIFY_SOCKET
// can't accidentally send sd_notify messages that systemd rejects under
// NotifyAccess=main and spam syslog.
var (
	notifyAddr   = os.Getenv("NOTIFY_SOCKET")
	watchdogUSec = os.Getenv("WATCHDOG_USEC")
)

func init() {
	_ = os.Unsetenv("NOTIFY_SOCKET")
	_ = os.Unsetenv("WATCHDOG_USEC")
	_ = os.Unsetenv("WATCHDOG_PID")
}

// notifySocket returns the systemd notify socket connection if NOTIFY_SOCKET was
// set when the process started. Returns (nil, nil) when the daemon is not
// running under a notify-capable systemd, so callers can no-op silently.
func notifySocket() (*net.UnixConn, error) {
	addr := notifyAddr
	if addr == "" {
		return nil, nil
	}
	if addr[0] == '@' {
		addr = "\x00" + addr[1:]
	}
	return net.DialUnix("unixgram", nil, &net.UnixAddr{Name: addr, Net: "unixgram"})
}

// SDNotifyReady tells systemd the daemon has finished startup. Safe to call
// when not under systemd.
func SDNotifyReady() {
	c, err := notifySocket()
	if err != nil || c == nil {
		return
	}
	defer c.Close()
	_, _ = c.Write([]byte("READY=1\n"))
}

// RunWatchdog periodically pings the systemd watchdog. It reads
// WATCHDOG_USEC and pings at half that interval. No-ops if the daemon is
// not under a watchdog-enabled unit. Returns when ctx is done.
func RunWatchdog(ctx context.Context) {
	usec := watchdogUSec
	if usec == "" {
		return
	}
	v, err := strconv.ParseInt(usec, 10, 64)
	if err != nil || v <= 0 {
		return
	}
	interval := time.Duration(v) * time.Microsecond / 2
	if interval < time.Second {
		interval = time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c, err := notifySocket()
			if err != nil || c == nil {
				continue
			}
			_, _ = c.Write([]byte("WATCHDOG=1\n"))
			_ = c.Close()
		}
	}
}
