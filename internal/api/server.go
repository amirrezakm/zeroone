package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/amirrezakm/zeroone/internal/analytics"
	"github.com/amirrezakm/zeroone/internal/audit"
	"github.com/amirrezakm/zeroone/internal/auth"
	"github.com/amirrezakm/zeroone/internal/bandwidth"
	"github.com/amirrezakm/zeroone/internal/enforce"
	"github.com/amirrezakm/zeroone/internal/events"
	"github.com/amirrezakm/zeroone/internal/failover"
	"github.com/amirrezakm/zeroone/internal/firewall"
	"github.com/amirrezakm/zeroone/internal/links"
	"github.com/amirrezakm/zeroone/internal/metrics"
	"github.com/amirrezakm/zeroone/internal/monitor"
	"github.com/amirrezakm/zeroone/internal/presence"
	"github.com/amirrezakm/zeroone/internal/relay"
	"github.com/amirrezakm/zeroone/internal/sessions"
	"github.com/amirrezakm/zeroone/internal/snapshots"
	"github.com/amirrezakm/zeroone/internal/stack"
	"github.com/amirrezakm/zeroone/internal/subscription"
	"github.com/amirrezakm/zeroone/internal/tunnel"
	"github.com/amirrezakm/zeroone/internal/usage"
	"github.com/amirrezakm/zeroone/internal/xray"
)

type Options struct {
	Metrics         *metrics.Store
	Events          *events.Broker
	Audit           *audit.Log
	Snapshots       *snapshots.Store
	Presence        *presence.Tracker
	Destinations    *analytics.Aggregator
	RelayStore      *relay.Store
	RelaySupervisor *relay.Supervisor
}

type Server struct {
	cfg             stack.Config
	configPath      string
	allowApply      bool
	metrics         *metrics.Store
	events          *events.Broker
	audit           *audit.Log
	snapshots       *snapshots.Store
	presence        *presence.Tracker
	destinations    *analytics.Aggregator
	relayStore      *relay.Store
	relaySupervisor *relay.Supervisor
}

func NewServer(cfg stack.Config, configPath string, allowApply bool) http.Handler {
	return NewServerWithOptions(cfg, configPath, allowApply, Options{})
}

func NewServerWithOptions(cfg stack.Config, configPath string, allowApply bool, opts Options) http.Handler {
	s := &Server{cfg: cfg, configPath: configPath, allowApply: allowApply,
		metrics: opts.Metrics, events: opts.Events, audit: opts.Audit, snapshots: opts.Snapshots,
		presence: opts.Presence, destinations: opts.Destinations,
		relayStore: opts.RelayStore, relaySupervisor: opts.RelaySupervisor}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("POST /api/test/connect", s.testConnect)
	mux.HandleFunc("GET /api/system", s.system)
	mux.HandleFunc("GET /api/users/activity", s.userActivity)
	mux.HandleFunc("GET /api/users/online", s.usersOnline)
	mux.HandleFunc("GET /api/config/summary", s.summary)
	mux.HandleFunc("PUT /api/client-endpoints", s.upsertClientEndpoint)
	mux.HandleFunc("DELETE /api/client-endpoints", s.deleteClientEndpoint)
	mux.HandleFunc("GET /api/client-endpoints/health", s.clientEndpointHealth)
	mux.HandleFunc("GET /api/xray/generated", s.generatedXray)
	mux.HandleFunc("GET /api/xray/live", s.liveXray)
	mux.HandleFunc("GET /api/xray/traffic", s.xrayTraffic)
	mux.HandleFunc("GET /api/xray/apply-plan", s.xrayApplyPlan)
	mux.HandleFunc("POST /api/xray/apply", s.xrayApply)
	mux.HandleFunc("GET /api/failover/decision", s.failoverDecision)
	mux.HandleFunc("GET /api/failover/history", s.failoverHistory)
	mux.HandleFunc("PUT /api/failover/mode", s.failoverMode)
	mux.HandleFunc("GET /api/usage", s.usage)
	mux.HandleFunc("POST /api/usage/sync", s.syncUsage)
	mux.HandleFunc("POST /api/usage/reset", s.resetUsage)
	mux.HandleFunc("GET /api/quota/plan", s.quotaPlan)
	mux.HandleFunc("POST /api/quota/apply", s.quotaApply)
	mux.HandleFunc("GET /api/bandwidth/plan", s.bandwidthPlan)
	mux.HandleFunc("POST /api/bandwidth/apply", s.bandwidthApply)
	mux.HandleFunc("POST /api/users", s.addUser)
	mux.HandleFunc("PUT /api/users", s.updateUser)
	mux.HandleFunc("DELETE /api/users", s.deleteUser)
	mux.HandleFunc("POST /api/users/ban", s.banUser)
	mux.HandleFunc("POST /api/users/unban", s.unbanUser)
	mux.HandleFunc("POST /api/users/quota", s.setUserQuota)
	mux.HandleFunc("POST /api/users/bandwidth", s.setUserBandwidth)
	mux.HandleFunc("POST /api/users/disconnect", s.disconnectUser)
	mux.HandleFunc("POST /api/direct-domains", s.addDirectDomain)
	mux.HandleFunc("DELETE /api/direct-domains", s.deleteDirectDomain)
	mux.HandleFunc("POST /api/socks", s.addSOCKS)
	mux.HandleFunc("PUT /api/socks", s.updateSOCKS)
	mux.HandleFunc("DELETE /api/socks", s.deleteSOCKS)
	mux.HandleFunc("GET /api/metrics", s.metricsHandler)
	mux.HandleFunc("GET /api/events", s.eventsHandler)
	mux.HandleFunc("GET /api/audit", s.auditHandler)
	mux.HandleFunc("GET /api/snapshots", s.snapshotsList)
	mux.HandleFunc("POST /api/snapshots", s.snapshotsCreate)
	mux.HandleFunc("POST /api/snapshots/rollback", s.snapshotsRollback)
	mux.HandleFunc("GET /api/xray/logs", s.xrayLogs)
	mux.HandleFunc("GET /api/users/bandwidth", s.userBandwidth)
	mux.HandleFunc("GET /api/analytics/destinations", s.destinationsTop)
	mux.HandleFunc("GET /api/tokens", s.tokensList)
	mux.HandleFunc("POST /api/tokens", s.tokenCreate)
	mux.HandleFunc("DELETE /api/tokens", s.tokenDelete)
	mux.HandleFunc("GET /api/notifications", s.notificationsGet)
	mux.HandleFunc("PUT /api/notifications", s.notificationsPut)
	mux.HandleFunc("POST /api/notifications/test", s.notificationsTest)
	mux.HandleFunc("GET /api/notifications/telegram/chats", s.telegramChats)
	mux.HandleFunc("GET /api/relay/config", s.relayConfigGet)
	mux.HandleFunc("PUT /api/relay/config", s.relayConfigPut)
	mux.HandleFunc("POST /api/relay/sites", s.relaySitesPost)
	mux.HandleFunc("PUT /api/relay/sites", s.relaySitesPut)
	mux.HandleFunc("DELETE /api/relay/sites", s.relaySitesDelete)
	mux.HandleFunc("GET /api/relay/status", s.relayStatus)
	mux.HandleFunc("POST /api/relay/test", s.relayTest)
	mux.HandleFunc("POST /api/relay/restart", s.relayRestart)
	mux.HandleFunc("GET /api/relay/logs", s.relayLogs)
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.logout)
	mux.HandleFunc("GET /api/me", s.me)
	mux.HandleFunc("GET /api/admins", s.adminsList)
	mux.HandleFunc("POST /api/admins", s.adminCreate)
	mux.HandleFunc("POST /api/admins/password", s.adminSetPassword)
	mux.HandleFunc("DELETE /api/admins", s.adminDelete)
	mux.HandleFunc("POST /api/users/periods", s.setUserPeriods)
	mux.HandleFunc("POST /api/users/max-sessions", s.setUserMaxSessions)
	// User-facing subscription + portal. Path-based token auth, no bearer
	// required. Sits above the SPA catch-all so /sub/{token} and /me/{token}
	// are matched first.
	subDeps := subscription.Deps{
		Config:        func() stack.Config { return s.cfg },
		LoadUsage:     func() (usage.UserState, error) { return usage.LoadUserState(s.cfg.Server.UserUsagePath) },
		PortalBaseURL: portalBaseURL,
	}
	mux.HandleFunc("GET /sub/", subscription.HandleSubscription(subDeps))
	mux.HandleFunc("GET /me/", subscription.HandlePortal(subDeps))

	mux.HandleFunc("GET /", s.ui)
	return s.tokenAuth(mux)
}

// portalBaseURL reconstructs the absolute URL the client used to reach
// zeroone, so generated subscription / portal links match the host
// the user knows about. nginx forwards Host via proxy_set_header, so
// r.Host is the original. Scheme is inferred: an IP host means raw HTTP
// access; a domain host means the request came through an HTTPS-fronting
// edge (runflare, pars-pack) which terminated TLS upstream.
func portalBaseURL(r *http.Request) string {
	scheme := "http"
	if p := r.Header.Get("X-Forwarded-Proto"); p == "https" {
		scheme = "https"
	} else if r.TLS != nil {
		scheme = "https"
	} else if h := r.Host; h != "" && !isLocalOrIPHost(h) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func isLocalOrIPHost(h string) bool {
	if i := strings.LastIndex(h, ":"); i > 0 {
		h = h[:i]
	}
	if h == "localhost" || strings.HasPrefix(h, "127.") {
		return true
	}
	// Bare IPv4: at least three dots and all numeric octets.
	dots := strings.Count(h, ".")
	if dots != 3 {
		return false
	}
	for _, part := range strings.Split(h, ".") {
		if _, err := strconv.Atoi(part); err != nil {
			return false
		}
	}
	return true
}

// publicAPIPaths bypass authentication entirely so the login flow and
// readiness checks remain usable when no session/token is present.
var publicAPIPaths = map[string]bool{
	"/api/login":  true,
	"/api/logout": true,
	"/api/me":     true,
	"/api/health": true,
}

func (s *Server) tokenAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if publicAPIPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		// Accept either a valid session cookie or a recognised Bearer token.
		// During the bootstrap window — admins not yet seeded — we fall back
		// to the Bearer-only behaviour so the operator can call POST
		// /api/admins via curl using an existing panel token.
		if username := auth.SessionFromRequest(r, s.cfg.Panel.SessionSecret); username != "" {
			next.ServeHTTP(w, r)
			return
		}
		hashes := make([]string, 0, len(s.cfg.Panel.Tokens))
		for _, t := range s.cfg.Panel.Tokens {
			hashes = append(hashes, t.Hash)
		}
		matched, ok := auth.LookupHash(r, hashes)
		if !ok {
			s.fail(w, http.StatusUnauthorized, fmt.Errorf("invalid bearer token"))
			return
		}
		if matched == "" && len(s.cfg.Panel.Admins) > 0 {
			// Admins exist but caller presented no credentials — require login.
			s.fail(w, http.StatusUnauthorized, fmt.Errorf("login required"))
			return
		}
		if matched != "" {
			go s.touchToken(matched)
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) touchToken(hash string) {
	for i := range s.cfg.Panel.Tokens {
		if s.cfg.Panel.Tokens[i].Hash == hash {
			s.cfg.Panel.Tokens[i].LastUsed = time.Now().Unix()
			_ = stack.Save(s.configPath, s.cfg)
			return
		}
	}
}

func (s *Server) actor(r *http.Request) string {
	if username := auth.SessionFromRequest(r, s.cfg.Panel.SessionSecret); username != "" {
		return username
	}
	if u, _, ok := r.BasicAuth(); ok && u != "" {
		return u
	}
	if a := r.Header.Get("X-Forwarded-User"); a != "" {
		return a
	}
	return "anonymous"
}

func (s *Server) recordAudit(actor, action, target string, data map[string]any) {
	if s.audit != nil {
		_ = s.audit.Write(actor, action, target, data)
	}
	if s.events != nil {
		ev := map[string]any{"actor": actor, "action": action, "target": target}
		for k, v := range data {
			ev[k] = v
		}
		s.events.Publish("audit", ev)
	}
}

func GenerateXrayForCLI(cfg stack.Config) any { return xray.Generate(cfg) }

func (s *Server) write(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func (s *Server) fail(w http.ResponseWriter, code int, err error) {
	w.WriteHeader(code)
	s.write(w, map[string]any{"ok": false, "error": err.Error()})
}

func (s *Server) save(w http.ResponseWriter) bool {
	if err := stack.Save(s.configPath, s.cfg); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

func (s *Server) currentConfig(w http.ResponseWriter) (stack.Config, bool) {
	cfg, err := stack.Load(s.configPath)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, fmt.Errorf("load current config: %w", err))
		return stack.Config{}, false
	}
	return *cfg, true
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.currentConfig(w)
	if !ok {
		return
	}
	checks := tunnel.CheckAll(r.Context(), cfg.Tunnels, cfg.Failover.ProbeTargets())
	s.write(w, map[string]any{"ok": true, "generated_at": time.Now().Format(time.RFC3339), "tunnels": checks})
}

func (s *Server) system(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.currentConfig(w)
	if !ok {
		return
	}
	s.write(w, monitor.SystemSnapshot(cfg))
}

func (s *Server) testConnect(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.currentConfig(w)
	if !ok {
		return
	}
	var req tunnel.ConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, 400, err)
		return
	}
	switch req.Route {
	case "", "direct":
		req.Route = "direct"
	case "priority-upstream":
		if cfg.Xray.Outbounds.Fallback.Address == "" || cfg.Xray.Outbounds.Fallback.Port == 0 {
			s.fail(w, 400, fmt.Errorf("fallback outbound is not configured"))
			return
		}
		req.Target = cfg.Xray.Outbounds.Fallback.Address
		req.Port = cfg.Xray.Outbounds.Fallback.Port
	case "tun0", "tun1":
		req.Interface = req.Route
	default:
		found := false
		for _, t := range cfg.Tunnels {
			if req.Route == t.Name || req.Route == t.Interface {
				req.Route = t.Interface
				req.Interface = t.Interface
				found = true
				break
			}
		}
		if !found {
			s.fail(w, 400, fmt.Errorf("unknown route %q", req.Route))
			return
		}
	}
	s.write(w, tunnel.TestConnect(r.Context(), req))
}

func (s *Server) usersOnline(w http.ResponseWriter, r *http.Request) {
	seconds := 300
	if v := r.URL.Query().Get("seconds"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 3600 {
			seconds = n
		}
	}
	snap, err := monitor.Online(r.Context(), nil, seconds, s.xrayInboundPorts())
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.write(w, snap)
}

// xrayInboundPorts returns every TCP port the panel-known xray inbounds
// listen on, used by `ss` queries that map peer IPs to active sessions
// and by sessions.KillByPeerIPs when enforcing limits. Includes the xhttp
// inbound (:3002 by default) — without it, the active-session count is
// always zero on this stack because that's where nginx hands off xhttp
// traffic from external CDNs.
func (s *Server) xrayInboundPorts() []int {
	ports := []int{}
	if p := s.cfg.Xray.Inbounds.VLESSWSPort; p != 0 {
		ports = append(ports, p)
	}
	if p := s.cfg.Xray.Inbounds.VLESSXHTTPPort; p != 0 {
		ports = append(ports, p)
	}
	for _, sock := range s.cfg.Xray.Inbounds.PublicSOCKS {
		if sock.Port != 0 {
			ports = append(ports, sock.Port)
		}
	}
	for _, u := range s.cfg.Xray.Users {
		if u.BandwidthPort != 0 {
			ports = append(ports, u.BandwidthPort)
		}
	}
	return ports
}

func (s *Server) userActivity(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	if email == "" {
		s.fail(w, 400, fmt.Errorf("email is required"))
		return
	}
	items, err := monitor.RecentActivity(r.Context(), nil, email, 180, 40)
	if err != nil {
		s.fail(w, 500, err)
		return
	}
	s.write(w, map[string]any{"email": email, "items": items})
}

func (s *Server) failoverHistory(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.currentConfig(w)
	if !ok {
		return
	}
	path := cfg.Server.FailoverHistoryPath
	entries, err := failover.LoadHistory(path)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.write(w, map[string]any{"ok": true, "entries": entries, "retention_hours": int(failover.DefaultHistoryRetention.Hours())})
}

func (s *Server) failoverDecision(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.currentConfig(w)
	if !ok {
		return
	}
	checks := tunnel.CheckAll(r.Context(), cfg.Tunnels, cfg.Failover.ProbeTargets())
	state, err := failover.LoadState(cfg.Server.FailoverStatePath)
	if err != nil {
		s.fail(w, 500, err)
		return
	}
	decision, nextState := failover.Decide(cfg, state, checks, time.Now())
	s.write(w, map[string]any{
		"checks":           checks,
		"decision":         decision,
		"state":            state,
		"next_state":       nextState,
		"mode":             cfg.Failover.EffectiveMode(),
		"preferred_tunnel": cfg.Failover.PreferredTunnel,
	})
}

func (s *Server) failoverMode(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.currentConfig(w)
	if !ok {
		return
	}
	var req struct {
		Mode            string `json:"mode"`
		PreferredTunnel string `json:"preferred_tunnel"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	// Capture the active outbound AND the mode+preferred BEFORE the mutation
	// so we can report what actually changed (mode change, preferred change,
	// or interface change — any of these is worth recording).
	priorMode := failover.CurrentMode(cfg)
	priorFailoverMode := cfg.Failover.EffectiveMode()
	priorPreferredTunnel := cfg.Failover.PreferredTunnel
	if err := cfg.SetFailoverMode(req.Mode, req.PreferredTunnel); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	// Compute desired interface for the new mode, ignoring cooldown/confirmation:
	// user-initiated switches feel instant, only automatic switches debounce.
	checks := tunnel.CheckAll(r.Context(), cfg.Tunnels, cfg.Failover.ProbeTargets())
	// For manual/preferred mode, the user picked a specific tunnel — bind to
	// it directly. DesiredMode's "stick where put" semantic is for the
	// background loop, not for explicit user intent. If the chosen tunnel is
	// unhealthy, fall over (don't silently drop traffic on a dead pin).
	desired := failover.DesiredMode(cfg, checks)
	if cfg.Failover.EffectiveMode() != stack.FailoverModeAuto && cfg.Failover.PreferredTunnel != "" {
		for _, t := range cfg.Tunnels {
			if t.Name != cfg.Failover.PreferredTunnel {
				continue
			}
			pinned := failover.Mode{OutboundTag: cfg.Xray.Outbounds.Proxy.Tag, Interface: t.Interface}
			healthy := false
			for _, c := range checks {
				if c.Interface == t.Interface && c.Healthy {
					healthy = true
					break
				}
			}
			if healthy {
				desired = pinned
			}
			// If unhealthy, leave `desired` as whatever DesiredMode returned
			// (firstHealthyMode for both manual and preferred when chosen is
			// down), so the user doesn't get pinned to a dead tunnel.
			break
		}
	}
	failover.ApplyMode(&cfg, desired)
	if err := stack.Save(s.configPath, cfg); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	s.cfg = cfg

	// Reset the failover loop state so its confirmation counter doesn't fight
	// the manual switch, and stamp LastChange so subsequent auto switches still
	// respect cooldown.
	if cfg.Server.FailoverStatePath != "" {
		_ = failover.SaveState(cfg.Server.FailoverStatePath, failover.State{LastChangeUnix: time.Now().Unix()})
	}

	applied := false
	var applyErr error
	if s.allowApply {
		if _, err := (xray.Manager{}).Apply(r.Context(), cfg); err != nil {
			applyErr = err
		} else {
			applied = true
		}
	}

	// Always record a history entry for user-initiated changes (intent + outcome),
	// even when the xray apply fails. Failed apply still mutated stack.json, so
	// without this entry the failure would be invisible in the history view.
	if cfg.Server.FailoverHistoryPath != "" {
		newMode := cfg.Failover.EffectiveMode()
		newPreferred := cfg.Failover.PreferredTunnel
		interfaceChanged := priorMode != desired
		modeChanged := priorFailoverMode != newMode
		preferredChanged := priorPreferredTunnel != newPreferred
		if interfaceChanged || modeChanged || preferredChanged || applyErr != nil {
			// Build a reason that calls out what shifted, so an entry with
			// from==to (e.g. mode change only) is still informative.
			var parts []string
			if modeChanged {
				parts = append(parts, fmt.Sprintf("%s→%s", priorFailoverMode, newMode))
			} else {
				parts = append(parts, "mode="+newMode)
			}
			if preferredChanged {
				prev := priorPreferredTunnel
				if prev == "" {
					prev = "(none)"
				}
				next := newPreferred
				if next == "" {
					next = "(none)"
				}
				parts = append(parts, fmt.Sprintf("preferred: %s→%s", prev, next))
			} else if newPreferred != "" {
				parts = append(parts, "preferred="+newPreferred)
			}
			entry := failover.Entry{
				T:      time.Now().Unix(),
				From:   priorMode,
				To:     desired,
				Reason: "manual: " + strings.Join(parts, " "),
			}
			if applyErr != nil {
				entry.Error = "xray apply failed: " + applyErr.Error()
			}
			_ = failover.AppendHistory(cfg.Server.FailoverHistoryPath, entry, failover.DefaultHistoryRetention)
		}
	}

	if applyErr != nil {
		s.recordAudit(s.actor(r), "failover.mode.apply_failed", cfg.Failover.PreferredTunnel, map[string]any{"mode": cfg.Failover.EffectiveMode(), "error": applyErr.Error()})
		s.fail(w, http.StatusInternalServerError, fmt.Errorf("xray apply failed: %w", applyErr))
		return
	}
	s.recordAudit(s.actor(r), "failover.mode.set", cfg.Failover.PreferredTunnel, map[string]any{
		"mode":      cfg.Failover.EffectiveMode(),
		"interface": desired.Interface,
		"applied":   applied,
	})

	s.write(w, map[string]any{
		"ok":               true,
		"mode":             cfg.Failover.EffectiveMode(),
		"preferred_tunnel": cfg.Failover.PreferredTunnel,
		"effective":        desired,
		"applied":          applied,
	})
}

func (s *Server) summary(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.currentConfig(w)
	if !ok {
		return
	}
	// Pull cumulative usage totals so the summary already carries them — saves
	// a second round-trip from the panel and keeps the table sorted on usage.
	usageTotals := map[string]int64{}
	dailyTotals := map[string]int64{}
	weeklyTotals := map[string]int64{}
	monthlyTotals := map[string]int64{}
	periodMeta := map[string]usage.PeriodMeta{}
	if state, err := usage.LoadUserState(cfg.Server.UserUsagePath); err == nil {
		for email, p := range state.Totals {
			usageTotals[email] = p.Uplink + p.Downlink
		}
		for email, p := range state.Daily {
			dailyTotals[email] = p.Uplink + p.Downlink
		}
		for email, p := range state.Weekly {
			weeklyTotals[email] = p.Uplink + p.Downlink
		}
		for email, p := range state.Monthly {
			monthlyTotals[email] = p.Uplink + p.Downlink
		}
		periodMeta = state.Periods
	}
	var presenceMap map[string]presence.Entry
	if s.presence != nil {
		presenceMap = s.presence.Snapshot()
	}

	// Precompute the portal/sub URL hostnames once — same list applies to
	// every user (only the token differs). Origin is plain HTTP (no TLS at
	// origin); enabled TLS-fronted client_endpoints are added as https://.
	// Hosts are de-duplicated so multiple endpoints sharing a hostname
	// (e.g. pars-pack + pars-pack-splithttp) don't repeat.
	portalHosts := []string{}
	if cfg.Server.PublicIP != "" {
		portalHosts = append(portalHosts, "http://"+cfg.Server.PublicIP)
	}
	seenHost := map[string]bool{}
	for _, ep := range cfg.Server.ClientEndpoints {
		if !ep.Enabled || !ep.TLS || ep.Host == "" || seenHost[ep.Host] {
			continue
		}
		seenHost[ep.Host] = true
		portalHosts = append(portalHosts, "https://"+ep.Host)
	}

	userViews := make([]map[string]any, 0, len(cfg.Xray.Users))
	for _, u := range cfg.Xray.Users {
		view := map[string]any{
			"email":               u.Email,
			"uuid":                u.UUID,
			"enabled":             u.Enabled,
			"banned_until":        u.BannedUntil,
			"quota_bytes":         u.QuotaBytes,
			"download_mbps":       u.DownloadMbps,
			"upload_mbps":         u.UploadMbps,
			"bandwidth_port":      u.BandwidthPort,
			"daily_quota_bytes":   u.DailyQuotaBytes,
			"weekly_quota_bytes":  u.WeeklyQuotaBytes,
			"monthly_quota_bytes": u.MonthlyQuotaBytes,
			"daily_reset_hhmm":    u.EffectiveDailyResetHHMM(),
			"max_sessions":        u.MaxSessions,
			"links":               links.VLESS(cfg, u),
			"used_bytes":          usageTotals[u.Email],
			"used_daily_bytes":    dailyTotals[u.Email],
			"used_weekly_bytes":   weeklyTotals[u.Email],
			"used_monthly_bytes":  monthlyTotals[u.Email],
			"sub_token":           u.SubToken,
		}
		if m, ok := periodMeta[u.Email]; ok {
			view["daily_reset_at"] = m.DailyResetAt
			view["weekly_reset_at"] = m.WeeklyResetAt
			view["monthly_reset_at"] = m.MonthlyResetAt
		}
		// Build per-user portal + subscription URLs against every host the
		// admin can hand out. Empty list when sub_token is missing (which
		// shouldn't happen post-backfill, but be defensive).
		if u.SubToken != "" {
			portals := make([]map[string]string, 0, len(portalHosts))
			for _, base := range portalHosts {
				portals = append(portals, map[string]string{
					"host":   base,
					"portal": base + "/me/" + u.SubToken,
					"sub":    base + "/sub/" + u.SubToken,
				})
			}
			view["portal_urls"] = portals
		}
		if p, ok := presenceMap[u.Email]; ok {
			view["last_seen"] = p.LastSeen
			view["last_ip"] = p.LastIP
		}
		userViews = append(userViews, view)
	}
	socksViews := make([]map[string]any, 0, len(cfg.Xray.Inbounds.PublicSOCKS))
	for _, socks := range cfg.Xray.Inbounds.PublicSOCKS {
		socksViews = append(socksViews, map[string]any{
			"name":     socks.Name,
			"listen":   socks.Listen,
			"port":     socks.Port,
			"username": socks.Username,
			"password": socks.Password,
			"links":    links.SOCKS(cfg, socks),
		})
	}
	s.write(w, map[string]any{
		"public_ip":        cfg.Server.PublicIP,
		"users":            len(cfg.Xray.Users),
		"socks":            len(cfg.Xray.Inbounds.PublicSOCKS),
		"user_items":       userViews,
		"socks_items":      socksViews,
		"direct_domains":   cfg.Xray.Routing.DirectDomains,
		"block_domains":    cfg.Xray.Routing.BlockDomains,
		"manual_blocks":    cfg.Xray.Routing.ManualBlockDomains,
		"client_endpoints": cfg.Server.ClientEndpoints,
		"tunnels":          cfg.Tunnels,
		"failover":         cfg.Failover,
		"allow_apply":      s.allowApply,
	})
}

func (s *Server) upsertClientEndpoint(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.currentConfig(w)
	if !ok {
		return
	}
	var req stack.ClientEndpoint
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	if err := cfg.UpsertClientEndpoint(req); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	if err := stack.Save(s.configPath, cfg); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	s.cfg = cfg
	s.recordAudit(s.actor(r), "client_endpoint.upsert", req.Name, map[string]any{"host": req.Host, "enabled": req.Enabled})
	s.write(w, map[string]any{"ok": true, "client_endpoints": cfg.Server.ClientEndpoints})
}

func (s *Server) deleteClientEndpoint(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.currentConfig(w)
	if !ok {
		return
	}
	name := r.URL.Query().Get("name")
	if err := cfg.DeleteClientEndpoint(name); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	if err := stack.Save(s.configPath, cfg); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	s.cfg = cfg
	s.recordAudit(s.actor(r), "client_endpoint.delete", name, nil)
	s.write(w, map[string]any{"ok": true, "client_endpoints": cfg.Server.ClientEndpoints})
}

type clientEndpointHealthResult struct {
	Name          string `json:"name"`
	Host          string `json:"host"`
	Network       string `json:"network"`
	URL           string `json:"url"`
	Enabled       bool   `json:"enabled"`
	OK            bool   `json:"ok"`
	StatusCode    int    `json:"status_code,omitempty"`
	LatencyMS     int64  `json:"latency_ms,omitempty"`
	Error         string `json:"error,omitempty"`
	LandingURL    string `json:"landing_url,omitempty"`
	LandingOK     bool   `json:"landing_ok"`
	LandingStatus int    `json:"landing_status,omitempty"`
	CheckedAt     string `json:"checked_at"`
}

func (s *Server) clientEndpointHealth(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.currentConfig(w)
	if !ok {
		return
	}
	results := make([]clientEndpointHealthResult, 0, len(cfg.Server.ClientEndpoints))
	for _, ep := range cfg.Server.ClientEndpoints {
		results = append(results, probeClientEndpoint(r.Context(), ep))
	}
	s.write(w, map[string]any{"ok": true, "generated_at": time.Now().Format(time.RFC3339), "endpoints": results})
}

func probeClientEndpoint(ctx context.Context, ep stack.ClientEndpoint) clientEndpointHealthResult {
	checkedAt := time.Now().Format(time.RFC3339)
	result := clientEndpointHealthResult{
		Name:    ep.Name,
		Host:    ep.Host,
		Network: ep.Network,
		URL:     clientEndpointURL(ep, ep.Path),
		Enabled: ep.Enabled,
		LandingURL: clientEndpointURL(stack.ClientEndpoint{
			Host: ep.Host,
			Port: ep.Port,
			TLS:  ep.TLS,
		}, "/"),
		CheckedAt: checkedAt,
	}
	if !ep.Enabled {
		result.Error = "endpoint disabled"
		return result
	}

	pathStatus, pathLatency, pathErr := httpProbe(ctx, result.URL)
	result.StatusCode = pathStatus
	result.LatencyMS = pathLatency
	if pathErr != nil {
		result.Error = pathErr.Error()
		return result
	}

	landingStatus, _, landingErr := httpProbe(ctx, result.LandingURL)
	result.LandingStatus = landingStatus
	result.LandingOK = landingErr == nil && landingStatus >= 200 && landingStatus < 400

	// A normal browser-style GET to an XHTTP endpoint commonly returns 404/405,
	// while CDN/origin failures surface as 5xx. Treat non-5xx as reachable.
	result.OK = pathStatus > 0 && pathStatus < 500
	if landingErr != nil || !result.LandingOK {
		if result.Error == "" {
			if landingErr != nil {
				result.Error = "landing probe: " + landingErr.Error()
			} else {
				result.Error = fmt.Sprintf("landing probe returned HTTP %d", landingStatus)
			}
		}
	}
	return result
}

func clientEndpointURL(ep stack.ClientEndpoint, path string) string {
	scheme := "http"
	if ep.TLS {
		scheme = "https"
	}
	host := ep.Host
	if ep.Port != 0 && (scheme != "https" || ep.Port != 443) && (scheme != "http" || ep.Port != 80) {
		host = fmt.Sprintf("%s:%d", host, ep.Port)
	}
	if path == "" || path[0] != '/' {
		path = "/" + path
	}
	return fmt.Sprintf("%s://%s%s", scheme, host, path)
}

func httpProbe(ctx context.Context, rawURL string) (int, int64, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodHead, rawURL, nil)
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Zeroone-Health/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Cache-Control", "no-cache")

	client := &http.Client{
		Timeout: 6 * time.Second,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ForceAttemptHTTP2:     false,
			ResponseHeaderTimeout: 5 * time.Second,
			DialContext: (&net.Dialer{
				Timeout:   4 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		},
	}
	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return 0, latency, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, latency, nil
}

func (s *Server) generatedXray(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.currentConfig(w)
	if !ok {
		return
	}
	s.write(w, xray.Generate(cfg))
}

func (s *Server) xrayTraffic(w http.ResponseWriter, r *http.Request) {
	if s.metrics == nil {
		s.write(w, map[string]any{"ok": true, "outbounds": map[string]any{}, "inbounds": map[string]any{}})
		return
	}
	snap := s.metrics.LatestStats()
	// Compute per-tag rate from the most recent fine-grained sample.
	// rates: outbound_<tag>_<dir>_bps; inboundRates: inbound_<tag>_<dir>_bps.
	rates := map[string]map[string]float64{}
	inboundRates := map[string]map[string]float64{}
	collect := func(prefix string, dst map[string]map[string]float64, k string, v float64) {
		if !strings.HasPrefix(k, prefix) {
			return
		}
		rest := k[len(prefix):]
		for _, suffix := range []string{"_uplink_bps", "_downlink_bps"} {
			if strings.HasSuffix(rest, suffix) {
				tag := rest[:len(rest)-len(suffix)]
				if dst[tag] == nil {
					dst[tag] = map[string]float64{}
				}
				dst[tag][strings.TrimPrefix(suffix, "_")] = v
			}
		}
	}
	if recent := s.metrics.Fine.Snapshot(time.Now().Unix() - 600); len(recent) > 0 {
		latest := recent[len(recent)-1]
		for k, v := range latest.Values {
			collect("outbound_", rates, k, v)
			collect("inbound_", inboundRates, k, v)
		}
	}
	s.write(w, map[string]any{
		"ok":            true,
		"updated_at":    snap.UpdatedAt,
		"inbounds":      snap.Inbounds,
		"outbounds":     snap.Outbounds,
		"rates":         rates,
		"inbound_rates": inboundRates,
	})
}

func (s *Server) liveXray(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.currentConfig(w)
	if !ok {
		return
	}
	b, err := os.ReadFile(cfg.Server.XrayConfigPath)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	var live any
	if err := json.Unmarshal(b, &live); err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.write(w, live)
}

func (s *Server) xrayApplyPlan(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.currentConfig(w)
	if !ok {
		return
	}
	m := xray.Manager{}
	plan, _, err := m.Plan(r.Context(), cfg)
	if err != nil {
		s.fail(w, 400, err)
		return
	}
	s.write(w, map[string]any{"ok": true, "valid": true, "config_path": cfg.Server.XrayConfigPath, "allow_apply": s.allowApply, "changed": plan.Changed, "backup_path": plan.BackupPath})
}

func (s *Server) xrayApply(w http.ResponseWriter, r *http.Request) {
	if !s.allowApply {
		s.fail(w, http.StatusForbidden, fmt.Errorf("apply is disabled; start with -allow-apply"))
		return
	}
	cfg, ok := s.currentConfig(w)
	if !ok {
		return
	}
	if s.snapshots != nil {
		if _, err := s.snapshots.Capture(s.configPath, cfg.Server.XrayConfigPath); err != nil {
			// Non-fatal: capture failure shouldn't block apply.
			s.recordAudit(s.actor(r), "snapshot.error", "", map[string]any{"error": err.Error()})
		}
	}
	plan, err := (xray.Manager{}).Apply(r.Context(), cfg)
	if err != nil {
		s.recordAudit(s.actor(r), "xray.apply.failed", "", map[string]any{"error": err.Error()})
		s.fail(w, 500, err)
		return
	}
	s.recordAudit(s.actor(r), "xray.apply", "", map[string]any{"changed": plan.Changed, "backup_path": plan.BackupPath})
	s.write(w, map[string]any{"ok": true, "plan": plan})
}

func (s *Server) usage(w http.ResponseWriter, r *http.Request) {
	state, err := usage.LoadUserState(s.cfg.Server.UserUsagePath)
	if err != nil {
		s.fail(w, 500, err)
		return
	}
	s.write(w, map[string]any{"updated_at": state.UpdatedAt, "users": usage.UserViews(state)})
}

func (s *Server) syncUsage(w http.ResponseWriter, r *http.Request) {
	raw, err := usage.QueryXrayUsers(r.Context(), nil, s.xrayAPIServer())
	if err != nil {
		s.fail(w, 500, err)
		return
	}
	state, err := usage.LoadUserState(s.cfg.Server.UserUsagePath)
	if err != nil {
		s.fail(w, 500, err)
		return
	}
	state = usage.SyncUsers(state, raw, time.Now())
	if err := usage.SaveUserState(s.cfg.Server.UserUsagePath, state); err != nil {
		s.fail(w, 500, err)
		return
	}
	s.recordAudit(s.actor(r), "usage.sync", "", map[string]any{"users": len(state.Totals)})
	s.write(w, map[string]any{"ok": true, "updated_at": state.UpdatedAt, "users": usage.UserViews(state)})
}

func (s *Server) resetUsage(w http.ResponseWriter, r *http.Request) {
	if !s.allowApply {
		s.fail(w, http.StatusForbidden, fmt.Errorf("usage reset is disabled; start with -allow-apply"))
		return
	}
	known := make([]string, 0, len(s.cfg.Xray.Users))
	for _, u := range s.cfg.Xray.Users {
		known = append(known, u.Email)
	}
	raw, err := usage.QueryXrayUsers(r.Context(), nil, s.xrayAPIServer())
	if err != nil {
		s.fail(w, 500, err)
		return
	}
	state, err := usage.LoadUserState(s.cfg.Server.UserUsagePath)
	if err != nil {
		s.fail(w, 500, err)
		return
	}
	state = usage.ResetUsers(state, raw, known, time.Now())
	if err := usage.SaveUserState(s.cfg.Server.UserUsagePath, state); err != nil {
		s.fail(w, 500, err)
		return
	}
	s.recordAudit(s.actor(r), "usage.reset", "", nil)
	s.write(w, map[string]any{"ok": true, "updated_at": state.UpdatedAt, "users": usage.UserViews(state)})
}

func (s *Server) quotaPlan(w http.ResponseWriter, r *http.Request) {
	state, err := usage.LoadUserState(s.cfg.Server.UserUsagePath)
	if err != nil {
		s.fail(w, 500, err)
		return
	}
	s.write(w, enforce.PlanQuota(s.cfg, state, time.Now()))
}

func (s *Server) quotaApply(w http.ResponseWriter, r *http.Request) {
	if !s.allowApply {
		s.fail(w, http.StatusForbidden, fmt.Errorf("quota apply is disabled; start with -allow-apply"))
		return
	}
	state, err := usage.LoadUserState(s.cfg.Server.UserUsagePath)
	if err != nil {
		s.fail(w, 500, err)
		return
	}
	plan := enforce.PlanQuota(s.cfg, state, time.Now())
	next := s.cfg
	if err := enforce.ApplyQuotaPlan(&next, plan); err != nil {
		s.fail(w, 500, err)
		return
	}
	if err := stack.Save(s.configPath, next); err != nil {
		s.fail(w, 500, err)
		return
	}
	s.cfg = next
	applyPlan, err := (xray.Manager{}).Apply(r.Context(), s.cfg)
	if err != nil {
		s.fail(w, 500, err)
		return
	}
	s.recordAudit(s.actor(r), "quota.apply", "", map[string]any{"actions": len(plan.Actions), "changed": applyPlan.Changed})
	s.write(w, map[string]any{"ok": true, "quota_plan": plan, "xray_apply": applyPlan})
}

func (s *Server) bandwidthPlan(w http.ResponseWriter, r *http.Request) {
	s.write(w, bandwidth.BuildPlan(s.cfg, s.allowApply))
}

func (s *Server) bandwidthApply(w http.ResponseWriter, r *http.Request) {
	if !s.allowApply {
		s.fail(w, http.StatusForbidden, fmt.Errorf("bandwidth apply is disabled; start with -allow-apply"))
		return
	}
	result, err := (bandwidth.Manager{}).Apply(r.Context(), s.cfg)
	if err != nil {
		s.fail(w, 500, err)
		return
	}
	s.recordAudit(s.actor(r), "bandwidth.apply", "", nil)
	s.write(w, map[string]any{"ok": true, "result": result})
}

func (s *Server) addUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
		UUID  string `json:"uuid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, 400, err)
		return
	}
	if err := s.cfg.AddUser(req.Email, req.UUID); err != nil {
		s.fail(w, 400, err)
		return
	}
	if !s.save(w) {
		return
	}
	var created stack.User
	for _, u := range s.cfg.Xray.Users {
		if u.Email == req.Email {
			created = u
			break
		}
	}
	s.recordAudit(s.actor(r), "user.create", req.Email, nil)
	s.write(w, map[string]any{"ok": true, "user": created, "links": links.VLESS(s.cfg, created)})
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldEmail string `json:"old_email"`
		Email    string `json:"email"`
		UUID     string `json:"uuid"`
		Enabled  bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, 400, err)
		return
	}
	if err := s.cfg.UpdateUser(req.OldEmail, req.Email, req.UUID, req.Enabled); err != nil {
		s.fail(w, 400, err)
		return
	}
	if !s.save(w) {
		return
	}
	s.recordAudit(s.actor(r), "user.update", req.Email, map[string]any{"enabled": req.Enabled})
	s.write(w, map[string]any{"ok": true})
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	if err := s.cfg.DeleteUser(email); err != nil {
		s.fail(w, 404, err)
		return
	}
	if !s.save(w) {
		return
	}
	s.recordAudit(s.actor(r), "user.delete", email, nil)
	s.write(w, map[string]any{"ok": true})
}

func (s *Server) banUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email   string `json:"email"`
		Minutes int    `json:"minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, 400, err)
		return
	}
	if req.Minutes <= 0 {
		req.Minutes = 60
	}
	until := time.Now().Add(time.Duration(req.Minutes) * time.Minute).Unix()
	if err := s.cfg.BanUser(req.Email, until); err != nil {
		s.fail(w, 400, err)
		return
	}
	if !s.save(w) {
		return
	}
	s.recordAudit(s.actor(r), "user.ban", req.Email, map[string]any{"minutes": req.Minutes, "until": until})
	s.write(w, map[string]any{"ok": true, "email": req.Email, "banned_until": until})
}

func (s *Server) unbanUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, 400, err)
		return
	}
	if err := s.cfg.UnbanUser(req.Email); err != nil {
		s.fail(w, 400, err)
		return
	}
	if !s.save(w) {
		return
	}
	s.recordAudit(s.actor(r), "user.unban", req.Email, nil)
	s.write(w, map[string]any{"ok": true, "email": req.Email})
}

func (s *Server) setUserQuota(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email      string `json:"email"`
		QuotaBytes int64  `json:"quota_bytes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, 400, err)
		return
	}
	if err := s.cfg.SetUserQuota(req.Email, req.QuotaBytes); err != nil {
		s.fail(w, 400, err)
		return
	}
	if !s.save(w) {
		return
	}
	s.recordAudit(s.actor(r), "user.quota", req.Email, map[string]any{"quota_bytes": req.QuotaBytes})
	s.write(w, map[string]any{"ok": true})
}

func (s *Server) setUserBandwidth(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email        string `json:"email"`
		DownloadMbps int    `json:"download_mbps"`
		UploadMbps   int    `json:"upload_mbps"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, 400, err)
		return
	}
	priorPort := 0
	for _, u := range s.cfg.Xray.Users {
		if u.Email == req.Email {
			priorPort = u.BandwidthPort
			break
		}
	}
	if err := s.cfg.SetUserBandwidth(req.Email, req.DownloadMbps, req.UploadMbps); err != nil {
		s.fail(w, 400, err)
		return
	}
	if !s.save(w) {
		return
	}
	port := 0
	for _, u := range s.cfg.Xray.Users {
		if u.Email == req.Email {
			port = u.BandwidthPort
			break
		}
	}
	resp := map[string]any{"ok": true, "bandwidth_port": port}
	if s.allowApply {
		xrayPlan, err := (xray.Manager{}).Apply(r.Context(), s.cfg)
		if err != nil {
			s.recordAudit(s.actor(r), "user.bandwidth.apply_failed", req.Email, map[string]any{"stage": "xray", "error": err.Error()})
			s.fail(w, 500, fmt.Errorf("xray apply failed: %w", err))
			return
		}
		bwResult, err := (bandwidth.Manager{}).Apply(r.Context(), s.cfg)
		if err != nil {
			s.recordAudit(s.actor(r), "user.bandwidth.apply_failed", req.Email, map[string]any{"stage": "bandwidth", "error": err.Error()})
			s.fail(w, 500, fmt.Errorf("bandwidth apply failed: %w", err))
			return
		}
		fw := firewall.UFW{}
		if priorPort > 0 && priorPort != port {
			if err := fw.Delete(r.Context(), priorPort); err != nil {
				s.recordAudit(s.actor(r), "user.bandwidth.firewall_warn", req.Email, map[string]any{"action": "delete", "port": priorPort, "error": err.Error()})
			}
		}
		if port > 0 {
			if err := fw.Allow(r.Context(), port); err != nil {
				s.recordAudit(s.actor(r), "user.bandwidth.apply_failed", req.Email, map[string]any{"stage": "firewall", "port": port, "error": err.Error()})
				s.fail(w, 500, fmt.Errorf("firewall allow %d/tcp failed: %w", port, err))
				return
			}
		}
		resp["xray_apply"] = xrayPlan
		resp["bandwidth_apply"] = bwResult
		resp["firewall_port"] = port
	}
	s.recordAudit(s.actor(r), "user.bandwidth", req.Email, map[string]any{"download_mbps": req.DownloadMbps, "upload_mbps": req.UploadMbps, "applied": s.allowApply, "prior_port": priorPort, "port": port})
	s.write(w, resp)
}

func (s *Server) disconnectUser(w http.ResponseWriter, r *http.Request) {
	if !s.allowApply {
		s.fail(w, http.StatusForbidden, fmt.Errorf("disconnect is disabled; start with -allow-apply"))
		return
	}
	var req struct {
		Email         string `json:"email"`
		WindowSeconds int    `json:"window_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, 400, err)
		return
	}
	if req.Email == "" {
		s.fail(w, 400, fmt.Errorf("email is required"))
		return
	}
	known := false
	for _, u := range s.cfg.Xray.Users {
		if u.Email == req.Email {
			known = true
			break
		}
	}
	if !known {
		s.fail(w, 404, fmt.Errorf("user %q not found", req.Email))
		return
	}
	window := req.WindowSeconds
	if window <= 0 || window > 3600 {
		window = 600
	}
	ports := s.xrayInboundPorts()
	snap, err := monitor.Online(r.Context(), nil, window, ports)
	if err != nil {
		s.fail(w, 500, err)
		return
	}
	var ips []string
	for _, u := range snap.Users {
		if u.Email == req.Email {
			ips = u.IPs
			break
		}
	}
	if len(ips) == 0 {
		s.recordAudit(s.actor(r), "user.disconnect", req.Email, map[string]any{"window": window, "killed": 0, "note": "no recent client IPs"})
		s.write(w, map[string]any{"ok": true, "killed": 0, "ips": []string{}, "ports": ports})
		return
	}
	result, err := sessions.KillByPeerIPs(r.Context(), nil, ports, ips)
	if err != nil {
		s.fail(w, 500, err)
		return
	}
	s.recordAudit(s.actor(r), "user.disconnect", req.Email, map[string]any{"window": window, "killed": result.Killed, "ports": ports, "ips": ips})
	s.write(w, map[string]any{"ok": true, "killed": result.Killed, "ips": ips, "ports": ports})
}

func (s *Server) addDirectDomain(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, 400, err)
		return
	}
	if err := s.cfg.AddDirectDomain(req.Domain); err != nil {
		s.fail(w, 400, err)
		return
	}
	if !s.save(w) {
		return
	}
	s.recordAudit(s.actor(r), "rule.direct.add", req.Domain, nil)
	applied, applyErr := s.tryApplyAfterRouteChange(r.Context())
	resp := map[string]any{"ok": true, "domain": req.Domain, "applied": applied}
	if applyErr != nil {
		resp["apply_error"] = applyErr.Error()
	}
	s.write(w, resp)
}

func (s *Server) deleteDirectDomain(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	if err := s.cfg.DeleteDirectDomain(domain); err != nil {
		s.fail(w, 400, err)
		return
	}
	if !s.save(w) {
		return
	}
	s.recordAudit(s.actor(r), "rule.direct.delete", domain, nil)
	applied, applyErr := s.tryApplyAfterRouteChange(r.Context())
	resp := map[string]any{"ok": true, "applied": applied}
	if applyErr != nil {
		resp["apply_error"] = applyErr.Error()
	}
	s.write(w, resp)
}

// tryApplyAfterRouteChange regenerates the live xray config so a routing
// mutation takes effect immediately. Skipped (silently) when the daemon
// wasn't started with -allow-apply; in that mode the panel surfaces the
// pending change via /api/xray/apply-plan and the operator runs apply.
func (s *Server) tryApplyAfterRouteChange(ctx context.Context) (bool, error) {
	if !s.allowApply {
		return false, nil
	}
	if _, err := (xray.Manager{}).Apply(ctx, s.cfg); err != nil {
		s.recordAudit("system", "xray.apply.auto.failed", "", map[string]any{"error": err.Error()})
		return false, err
	}
	s.recordAudit("system", "xray.apply.auto", "", nil)
	return true, nil
}

func (s *Server) addSOCKS(w http.ResponseWriter, r *http.Request) {
	var req stack.SOCKSInbound
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, 400, err)
		return
	}
	if err := s.cfg.AddSOCKS(req); err != nil {
		s.fail(w, 400, err)
		return
	}
	if !s.save(w) {
		return
	}
	s.recordAudit(s.actor(r), "socks.create", req.Name, map[string]any{"port": req.Port})
	s.write(w, map[string]any{"ok": true, "name": req.Name, "port": req.Port})
}

func (s *Server) updateSOCKS(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldUsername string `json:"old_username"`
		Name        string `json:"name"`
		Listen      string `json:"listen"`
		Port        int    `json:"port"`
		Username    string `json:"username"`
		Password    string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, 400, err)
		return
	}
	if req.Listen == "" {
		req.Listen = "0.0.0.0"
	}
	next := stack.SOCKSInbound{Name: req.Name, Listen: req.Listen, Port: req.Port, Username: req.Username, Password: req.Password}
	if err := s.cfg.UpdateSOCKS(req.OldUsername, next); err != nil {
		s.fail(w, 400, err)
		return
	}
	if !s.save(w) {
		return
	}
	s.recordAudit(s.actor(r), "socks.update", req.Username, nil)
	s.write(w, map[string]any{"ok": true})
}

func (s *Server) deleteSOCKS(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	if err := s.cfg.DeleteSOCKS(username); err != nil {
		s.fail(w, 404, err)
		return
	}
	if !s.save(w) {
		return
	}
	s.recordAudit(s.actor(r), "socks.delete", username, nil)
	s.write(w, map[string]any{"ok": true})
}

func (s *Server) ui(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Server.UIPath != "" && fileExists(filepath.Join(s.cfg.Server.UIPath, "index.html")) {
		s.serveUI(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8"><title>Zeroone</title></head><body><h1>Zeroone</h1><p>Go control plane is running.</p><script>fetch('/api/config/summary').then(r=>r.json()).then(d=>document.body.appendChild(document.createElement('pre')).textContent=JSON.stringify(d,null,2))</script></body></html>`))
}

func (s *Server) serveUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "" || r.URL.Path == "/" {
		http.ServeFile(w, r, filepath.Join(s.cfg.Server.UIPath, "index.html"))
		return
	}
	rel := strings.TrimPrefix(filepath.Clean(r.URL.Path), string(filepath.Separator))
	path := filepath.Join(s.cfg.Server.UIPath, rel)
	root, err := filepath.Abs(s.cfg.Server.UIPath)
	if err != nil {
		s.fail(w, 500, err)
		return
	}
	target, err := filepath.Abs(path)
	if err != nil {
		s.fail(w, 500, err)
		return
	}
	if target != root && !strings.HasPrefix(target, root+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}
	if st, err := os.Stat(target); err == nil && !st.IsDir() {
		http.ServeFile(w, r, target)
		return
	}
	http.ServeFile(w, r, filepath.Join(s.cfg.Server.UIPath, "index.html"))
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func (s *Server) xrayAPIServer() string {
	port := s.cfg.Xray.APIPort
	if port == 0 {
		port = 10085
	}
	return fmt.Sprintf("127.0.0.1:%d", port)
}
