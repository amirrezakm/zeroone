//go:build !linux

package tunnel

import (
	"syscall"
)

func bindControl(_ string) func(string, string, syscall.RawConn) error {
	return nil
}
