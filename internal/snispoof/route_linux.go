//go:build linux

package snispoof

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/amirrezakm/zeroone/internal/stack"
	"github.com/amirrezakm/zeroone/internal/system"
)

// rulePriority sits below the main-table rule (32766) and above local (0)
// so marked traffic is matched before the default routing decision.
const rulePriority = "12000"

// waitDevice polls for the tun interface to appear; tun2socks creates it.
func waitDevice(ctx context.Context, runner system.Runner, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if _, err := runner.Run(ctx, "ip", "link", "show", "dev", name); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("tun device %q did not appear within %s", name, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
}

// setupRoute programs the scoped policy route: packets stamped with
// FirewallMark are looked up in RouteTable, whose default route points at
// the tun. Only xray's sni-spoof outbound sets that mark, so nothing else
// is diverted and byedpi's unmarked egress never loops. Idempotent.
func setupRoute(ctx context.Context, runner system.Runner, c stack.SNISpoofConfig) error {
	tun := c.EffectiveTunName()
	table := strconv.Itoa(c.EffectiveTable())
	markHex := "0x" + strconv.FormatInt(int64(c.EffectiveMark()), 16)

	// Best-effort device config; tun2socks usually brings the link up.
	_, _ = runner.Run(ctx, "ip", "addr", "replace", c.EffectiveTunAddr(), "dev", tun)
	_, _ = runner.Run(ctx, "ip", "link", "set", "dev", tun, "mtu", strconv.Itoa(c.EffectiveMTU()))
	_, _ = runner.Run(ctx, "ip", "link", "set", "dev", tun, "up")

	if _, err := runner.Run(ctx, "ip", "route", "replace", "default", "dev", tun, "table", table); err != nil {
		return fmt.Errorf("ip route replace default dev %s table %s: %w", tun, table, err)
	}
	// Remove any stale matching rules first so reloads don't stack duplicates.
	for i := 0; i < 4; i++ {
		if _, err := runner.Run(ctx, "ip", "rule", "del", "fwmark", markHex, "table", table); err != nil {
			break
		}
	}
	if _, err := runner.Run(ctx, "ip", "rule", "add", "fwmark", markHex, "table", table, "priority", rulePriority); err != nil {
		return fmt.Errorf("ip rule add fwmark %s table %s: %w", markHex, table, err)
	}
	return nil
}

// teardownRoute removes the rule(s) and flushes the table. Best-effort: the
// tun device itself is owned by tun2socks and vanishes when it stops.
func teardownRoute(ctx context.Context, runner system.Runner, c stack.SNISpoofConfig) {
	table := strconv.Itoa(c.EffectiveTable())
	markHex := "0x" + strconv.FormatInt(int64(c.EffectiveMark()), 16)
	for i := 0; i < 8; i++ {
		if _, err := runner.Run(ctx, "ip", "rule", "del", "fwmark", markHex, "table", table); err != nil {
			break
		}
	}
	_, _ = runner.Run(ctx, "ip", "route", "flush", "table", table)
}
