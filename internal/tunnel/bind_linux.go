//go:build linux

package tunnel

import (
	"fmt"
	"syscall"
)

func bindControl(iface string) func(string, string, syscall.RawConn) error {
	if iface == "" {
		return nil
	}
	return func(_, _ string, c syscall.RawConn) error {
		var sockErr error
		if err := c.Control(func(fd uintptr) {
			sockErr = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, iface)
		}); err != nil {
			return err
		}
		if sockErr != nil {
			return fmt.Errorf("bind %s: %w", iface, sockErr)
		}
		return nil
	}
}
