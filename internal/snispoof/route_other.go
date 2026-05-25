//go:build !linux

package snispoof

import (
	"context"
	"fmt"
	"time"

	"github.com/amirrezakm/zeroone/internal/stack"
	"github.com/amirrezakm/zeroone/internal/system"
)

// The tun + policy-routing plumbing relies on Linux `ip`/fwmark semantics.
// These stubs let the daemon build on other platforms (e.g. macOS dev) with
// the feature disabled.

func waitDevice(_ context.Context, _ system.Runner, _ string, _ time.Duration) error {
	return fmt.Errorf("sni-spoof tun routing is only supported on linux")
}

func setupRoute(_ context.Context, _ system.Runner, _ stack.SNISpoofConfig) error {
	return fmt.Errorf("sni-spoof tun routing is only supported on linux")
}

func teardownRoute(_ context.Context, _ system.Runner, _ stack.SNISpoofConfig) {}
