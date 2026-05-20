package metrics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/amirrezakm/zeroone/internal/monitor"
	"github.com/amirrezakm/zeroone/internal/stack"
	"github.com/amirrezakm/zeroone/internal/stats"
	"github.com/amirrezakm/zeroone/internal/tunnel"
	"github.com/amirrezakm/zeroone/internal/usage"
)

// Store holds two resolutions. Fine = 5s × 720 (1h). Coarse = 1m × 1440 (24h).
type Store struct {
	Fine   *Series
	Coarse *Series

	mu              sync.Mutex
	lastTunnel      map[string][2]int64 // name -> [rx, tx] from previous tick for rate calc
	lastOutbound    map[string]stats.Pair
	lastOutboundBps map[string][2]float64 // tag -> [uplink_bps, downlink_bps]
	lastInbound     map[string]stats.Pair
	lastInboundBps  map[string][2]float64
	lastStatsTS     int64
	latestStats     stats.Snapshot
	configRead      func() stack.Config
	failoverTag     func() string

	// Per-user bandwidth: deltas observed via the auto-sync ticker.
	// emailBps[email] = [uplink_bps, downlink_bps] from the last sync interval.
	lastUser   map[string]usage.Pair
	lastUserTS int64
	emailBps   map[string][2]float64
}

func NewStore(configRead func() stack.Config, failoverTag func() string) *Store {
	return &Store{
		Fine:            NewSeries(720, 5*time.Second),
		Coarse:          NewSeries(1440, time.Minute),
		lastTunnel:      map[string][2]int64{},
		lastOutbound:    map[string]stats.Pair{},
		lastOutboundBps: map[string][2]float64{},
		lastInbound:     map[string]stats.Pair{},
		lastInboundBps:  map[string][2]float64{},
		lastUser:        map[string]usage.Pair{},
		emailBps:        map[string][2]float64{},
		configRead:      configRead,
		failoverTag:     failoverTag,
	}
}

// ObserveUserStats accepts per-email cumulative byte counters from the usage
// syncer and converts them into [uplink_bps, downlink_bps] for the metrics
// stream. Counter resets (Xray restart) produce 0, not negative rates.
func (st *Store) ObserveUserStats(raw map[string]usage.Pair, now time.Time) {
	st.mu.Lock()
	defer st.mu.Unlock()
	prevTS := st.lastUserTS
	st.lastUserTS = now.Unix()
	if prevTS == 0 {
		// first sample — just baseline
		for k, v := range raw {
			st.lastUser[k] = v
		}
		return
	}
	elapsed := float64(now.Unix() - prevTS)
	if elapsed <= 0 {
		return
	}
	next := map[string][2]float64{}
	for email, cur := range raw {
		prev, ok := st.lastUser[email]
		st.lastUser[email] = cur
		if !ok {
			continue
		}
		dUp := float64(cur.Uplink-prev.Uplink) / elapsed
		dDn := float64(cur.Downlink-prev.Downlink) / elapsed
		if dUp < 0 {
			dUp = 0
		}
		if dDn < 0 {
			dDn = 0
		}
		next[email] = [2]float64{dUp, dDn}
	}
	st.emailBps = next
}

// LatestUserBps returns a copy of the most recent per-email bandwidth rates.
func (st *Store) LatestUserBps() map[string][2]float64 {
	st.mu.Lock()
	defer st.mu.Unlock()
	out := make(map[string][2]float64, len(st.emailBps))
	for k, v := range st.emailBps {
		out[k] = v
	}
	return out
}

// LatestStats returns the most recent xray-stats snapshot the collector
// captured. Empty snapshot before the first successful query.
func (st *Store) LatestStats() stats.Snapshot {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.latestStats
}

// Run samples in the background. Returns when ctx is done.
func (st *Store) Run(ctx context.Context) {
	fine := time.NewTicker(5 * time.Second)
	coarse := time.NewTicker(time.Minute)
	defer fine.Stop()
	defer coarse.Stop()
	st.tick(ctx, true) // prime
	for {
		select {
		case <-ctx.Done():
			return
		case <-fine.C:
			st.tick(ctx, true)
		case <-coarse.C:
			st.tick(ctx, false)
		}
	}
}

func (st *Store) tick(ctx context.Context, fineOnly bool) {
	cfg := st.configRead()
	sys := monitor.SystemSnapshot(cfg)
	values := map[string]float64{
		"cpu_pct":  sys.CPU.Percent,
		"ram_pct":  sys.RAM.Percent,
		"ram_used": float64(sys.RAM.UsedBytes),
	}

	st.mu.Lock()
	for _, t := range sys.Tunnels {
		prev, ok := st.lastTunnel[t.Name]
		st.lastTunnel[t.Name] = [2]int64{t.RXBytes, t.TXBytes}
		if !ok {
			continue
		}
		// Rate per second over the last 5s tick.
		dRx := float64(t.RXBytes-prev[0]) / 5
		dTx := float64(t.TXBytes-prev[1]) / 5
		if t.RXBytes < prev[0] {
			dRx = 0
		}
		if t.TXBytes < prev[1] {
			dTx = 0
		}
		values["tunnel_"+t.Name+"_rx_bps"] = dRx
		values["tunnel_"+t.Name+"_tx_bps"] = dTx
		values["tunnel_"+t.Name+"_rx_total"] = float64(t.RXBytes)
		values["tunnel_"+t.Name+"_tx_total"] = float64(t.TXBytes)
	}
	st.mu.Unlock()

	// Probe latency snapshot: best (lowest) latency among healthy tunnels.
	checks := tunnel.CheckAll(ctx, cfg.Tunnels, cfg.Failover.ProbeTargets())
	for _, c := range checks {
		if c.Healthy {
			values["tunnel_"+c.Name+"_latency_ms"] = float64(c.LatencyMS)
		}
		if c.Up {
			values["tunnel_"+c.Name+"_up"] = 1
		} else {
			values["tunnel_"+c.Name+"_up"] = 0
		}
	}

	if st.failoverTag != nil {
		tag := st.failoverTag()
		if tag == "proxy" {
			values["failover_proxy"] = 1
		} else {
			values["failover_proxy"] = 0
		}
	}

	// Outbound traffic-by-tag: query xray stats at most once a minute,
	// but emit the cached cumulative totals on every tick so charts have
	// a value at every X.
	now := time.Now().Unix()
	if now-st.lastStatsTS >= 60 {
		st.queryAndStoreXrayStats(ctx, cfg, values, now)
		st.lastStatsTS = now
	}
	st.mu.Lock()
	for tag, p := range st.lastOutbound {
		if _, alreadySet := values["outbound_"+tag+"_uplink_total"]; !alreadySet {
			values["outbound_"+tag+"_uplink_total"] = float64(p.Uplink)
			values["outbound_"+tag+"_downlink_total"] = float64(p.Downlink)
		}
	}
	for tag, bps := range st.lastOutboundBps {
		if _, alreadySet := values["outbound_"+tag+"_uplink_bps"]; !alreadySet {
			values["outbound_"+tag+"_uplink_bps"] = bps[0]
			values["outbound_"+tag+"_downlink_bps"] = bps[1]
		}
	}
	for tag, p := range st.lastInbound {
		if _, alreadySet := values["inbound_"+tag+"_uplink_total"]; !alreadySet {
			values["inbound_"+tag+"_uplink_total"] = float64(p.Uplink)
			values["inbound_"+tag+"_downlink_total"] = float64(p.Downlink)
		}
	}
	for tag, bps := range st.lastInboundBps {
		if _, alreadySet := values["inbound_"+tag+"_uplink_bps"]; !alreadySet {
			values["inbound_"+tag+"_uplink_bps"] = bps[0]
			values["inbound_"+tag+"_downlink_bps"] = bps[1]
		}
	}
	for email, bps := range st.emailBps {
		values["user_"+email+"_uplink_bps"] = bps[0]
		values["user_"+email+"_downlink_bps"] = bps[1]
	}
	st.mu.Unlock()

	sample := Sample{Timestamp: now, Values: values}
	st.Fine.Append(sample)
	if !fineOnly {
		st.Coarse.Append(sample)
	}
}

func (st *Store) queryAndStoreXrayStats(ctx context.Context, cfg stack.Config, values map[string]float64, now int64) {
	port := cfg.Xray.APIPort
	if port == 0 {
		port = 10085
	}
	server := fmt.Sprintf("127.0.0.1:%d", port)
	snap, err := stats.Query(ctx, nil, server)
	if err != nil {
		// Stats are best-effort: a failed query shouldn't kill the tick.
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	prevTS := st.latestStats.UpdatedAt
	st.latestStats = snap
	rate := func(prev, cur stats.Pair, elapsed float64) (float64, float64) {
		dUp := float64(cur.Uplink-prev.Uplink) / elapsed
		dDn := float64(cur.Downlink-prev.Downlink) / elapsed
		if dUp < 0 {
			dUp = 0
		}
		if dDn < 0 {
			dDn = 0
		}
		return dUp, dDn
	}
	for tag, p := range snap.Outbounds {
		values["outbound_"+tag+"_uplink_total"] = float64(p.Uplink)
		values["outbound_"+tag+"_downlink_total"] = float64(p.Downlink)
		if prev, ok := st.lastOutbound[tag]; ok && prevTS > 0 {
			if elapsed := float64(now - prevTS); elapsed > 0 {
				dUp, dDn := rate(prev, p, elapsed)
				values["outbound_"+tag+"_uplink_bps"] = dUp
				values["outbound_"+tag+"_downlink_bps"] = dDn
				st.lastOutboundBps[tag] = [2]float64{dUp, dDn}
			}
		}
		st.lastOutbound[tag] = p
	}
	for tag, p := range snap.Inbounds {
		values["inbound_"+tag+"_uplink_total"] = float64(p.Uplink)
		values["inbound_"+tag+"_downlink_total"] = float64(p.Downlink)
		if prev, ok := st.lastInbound[tag]; ok && prevTS > 0 {
			if elapsed := float64(now - prevTS); elapsed > 0 {
				dUp, dDn := rate(prev, p, elapsed)
				values["inbound_"+tag+"_uplink_bps"] = dUp
				values["inbound_"+tag+"_downlink_bps"] = dDn
				st.lastInboundBps[tag] = [2]float64{dUp, dDn}
			}
		}
		st.lastInbound[tag] = p
	}
}
