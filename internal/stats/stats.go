// Package stats taps Xray's stats API for outbound + inbound bytes-by-tag.
// Xray does not expose per-routing-rule counters, but every rule routes to
// an outbound (proxy / direct / block / fallback) and the per-outbound
// counters give us a meaningful "traffic by action" view: bytes blocked,
// bytes sent direct, bytes proxied, bytes via fallback tunnel.
package stats

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/system"
)

type Pair struct {
	Uplink   int64 `json:"uplink"`
	Downlink int64 `json:"downlink"`
}

type Snapshot struct {
	Inbounds  map[string]Pair `json:"inbounds"`
	Outbounds map[string]Pair `json:"outbounds"`
	UpdatedAt int64           `json:"updated_at"`
}

func Query(ctx context.Context, runner system.Runner, server string) (Snapshot, error) {
	if runner == nil {
		runner = system.ExecRunner{Timeout: 6 * time.Second}
	}
	out := Snapshot{
		Inbounds:  map[string]Pair{},
		Outbounds: map[string]Pair{},
		UpdatedAt: time.Now().Unix(),
	}
	for _, kind := range []string{"inbound", "outbound"} {
		res, err := runner.Run(ctx, "xray", "api", "statsquery", "--server="+server, "-pattern", kind+">>>")
		if err != nil {
			return out, fmt.Errorf("statsquery %s: %w: %s%s", kind, err, res.Stdout, res.Stderr)
		}
		var payload struct {
			Stat []struct {
				Name  string `json:"name"`
				Value int64  `json:"value"`
			} `json:"stat"`
			Stats []struct {
				Name  string `json:"name"`
				Value int64  `json:"value"`
			} `json:"stats"`
		}
		if err := json.Unmarshal([]byte(res.Stdout), &payload); err != nil {
			return out, err
		}
		dst := out.Outbounds
		if kind == "inbound" {
			dst = out.Inbounds
		}
		for _, s := range append(payload.Stat, payload.Stats...) {
			parts := strings.Split(s.Name, ">>>")
			// Format: <kind>>>><tag>>>>traffic>>>uplink|downlink
			if len(parts) < 4 || parts[0] != kind || parts[2] != "traffic" {
				continue
			}
			tag := parts[1]
			p := dst[tag]
			switch parts[3] {
			case "uplink":
				p.Uplink += s.Value
			case "downlink":
				p.Downlink += s.Value
			}
			dst[tag] = p
		}
	}
	return out, nil
}
