package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/relay"
	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
)

// relayView is the panel-facing config representation. AuthKey is redacted
// to a "set/unset" flag — we only return the real value if the operator
// hasn't set one yet (so empty state is obvious).
type relayView struct {
	Enabled       bool              `json:"enabled"`
	Listen        string            `json:"listen"`
	SOCKSPort     int               `json:"socks_port"`
	AuthKeySet    bool              `json:"auth_key_set"`
	ScriptURL     string            `json:"script_url"`
	DeploymentIDs []string          `json:"deployment_ids"`
	GoogleIP      string            `json:"google_ip"`
	FrontDomain   string            `json:"front_domain"`
	LogLevel      string            `json:"log_level"`
	Binary        string            `json:"binary"`
	ConfigPath    string            `json:"config_path"`
	SystemdUnit   string            `json:"systemd_unit"`
	OutboundTag   string            `json:"outbound_tag"`
	Sites         []stack.RelaySite `json:"sites"`
	InboundTags   []string          `json:"inbound_tags"`
	HostsMap      map[string]string `json:"hosts_map"`
	HealthProbe   string            `json:"health_probe"`
	Notes         string            `json:"notes"`
	Defaults      relayDefaults     `json:"defaults"`
}

type relayDefaults struct {
	Listen      string `json:"listen"`
	SOCKSPort   int    `json:"socks_port"`
	OutboundTag string `json:"outbound_tag"`
	Binary      string `json:"binary"`
	ConfigPath  string `json:"config_path"`
	HealthProbe string `json:"health_probe"`
	GoogleIP    string `json:"google_ip"`
	FrontDomain string `json:"front_domain"`
	LogLevel    string `json:"log_level"`
}

func toRelayView(c stack.RelayConfig) relayView {
	sites := append([]stack.RelaySite(nil), c.Sites...)
	if sites == nil {
		sites = []stack.RelaySite{}
	}
	return relayView{
		Enabled:       c.Enabled,
		Listen:        c.Listen,
		SOCKSPort:     c.SOCKSPort,
		AuthKeySet:    c.AuthKey != "",
		ScriptURL:     c.ScriptURL,
		DeploymentIDs: append([]string{}, c.DeploymentIDs...),
		GoogleIP:      c.GoogleIP,
		FrontDomain:   c.FrontDomain,
		LogLevel:      c.LogLevel,
		Binary:        c.Binary,
		ConfigPath:    c.ConfigPath,
		SystemdUnit:   c.SystemdUnit,
		OutboundTag:   c.OutboundTag,
		Sites:         sites,
		InboundTags:   append([]string{}, c.InboundTags...),
		HostsMap:      c.HostsMap,
		HealthProbe:   c.HealthProbe,
		Notes:         c.Notes,
		Defaults: relayDefaults{
			Listen:      stack.DefaultRelayListen,
			SOCKSPort:   stack.DefaultRelaySOCKSPort,
			OutboundTag: stack.DefaultRelayOutboundTag,
			Binary:      stack.DefaultRelayBinary,
			ConfigPath:  stack.DefaultRelayConfigPath,
			HealthProbe: stack.DefaultRelayHealthProbe,
			GoogleIP:    stack.DefaultRelayGoogleIP,
			FrontDomain: stack.DefaultRelayFrontDomain,
			LogLevel:    stack.DefaultRelayLogLevel,
		},
	}
}

func (s *Server) relayConfigGet(w http.ResponseWriter, r *http.Request) {
	s.write(w, map[string]any{"ok": true, "config": toRelayView(s.cfg.Relay)})
}

// relayConfigPut accepts a partial update; omitted fields keep their
// existing values. AuthKey is special-cased: an empty AuthKey keeps the
// existing one (so the panel can save other fields without re-typing it).
func (s *Server) relayConfigPut(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled       *bool             `json:"enabled,omitempty"`
		Listen        *string           `json:"listen,omitempty"`
		SOCKSPort     *int              `json:"socks_port,omitempty"`
		AuthKey       *string           `json:"auth_key,omitempty"`
		ScriptURL     *string           `json:"script_url,omitempty"`
		DeploymentIDs *[]string         `json:"deployment_ids,omitempty"`
		GoogleIP      *string           `json:"google_ip,omitempty"`
		FrontDomain   *string           `json:"front_domain,omitempty"`
		LogLevel      *string           `json:"log_level,omitempty"`
		Binary        *string           `json:"binary,omitempty"`
		ConfigPath    *string           `json:"config_path,omitempty"`
		SystemdUnit   *string           `json:"systemd_unit,omitempty"`
		OutboundTag   *string           `json:"outbound_tag,omitempty"`
		InboundTags   *[]string         `json:"inbound_tags,omitempty"`
		HostsMap      map[string]string `json:"hosts_map,omitempty"`
		HealthProbe   *string           `json:"health_probe,omitempty"`
		Notes         *string           `json:"notes,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	cur := s.cfg.Relay
	if req.Enabled != nil {
		cur.Enabled = *req.Enabled
	}
	if req.Listen != nil {
		cur.Listen = strings.TrimSpace(*req.Listen)
	}
	if req.SOCKSPort != nil {
		cur.SOCKSPort = *req.SOCKSPort
	}
	if req.AuthKey != nil && *req.AuthKey != "" {
		cur.AuthKey = *req.AuthKey
	}
	if req.ScriptURL != nil {
		cur.ScriptURL = strings.TrimSpace(*req.ScriptURL)
	}
	if req.DeploymentIDs != nil {
		cur.DeploymentIDs = dedupTrim(*req.DeploymentIDs)
	}
	if req.GoogleIP != nil {
		cur.GoogleIP = strings.TrimSpace(*req.GoogleIP)
	}
	if req.FrontDomain != nil {
		cur.FrontDomain = strings.TrimSpace(*req.FrontDomain)
	}
	if req.LogLevel != nil {
		cur.LogLevel = strings.TrimSpace(*req.LogLevel)
	}
	if req.Binary != nil {
		cur.Binary = strings.TrimSpace(*req.Binary)
	}
	if req.ConfigPath != nil {
		cur.ConfigPath = strings.TrimSpace(*req.ConfigPath)
	}
	if req.SystemdUnit != nil {
		cur.SystemdUnit = strings.TrimSpace(*req.SystemdUnit)
	}
	if req.OutboundTag != nil {
		cur.OutboundTag = strings.TrimSpace(*req.OutboundTag)
	}
	if req.InboundTags != nil {
		cur.InboundTags = dedupTrim(*req.InboundTags)
	}
	if req.HostsMap != nil {
		cur.HostsMap = req.HostsMap
	}
	if req.HealthProbe != nil {
		cur.HealthProbe = strings.TrimSpace(*req.HealthProbe)
	}
	if req.Notes != nil {
		cur.Notes = *req.Notes
	}
	prev := s.cfg.Relay
	s.cfg.Relay = cur
	if !s.save(w) {
		s.cfg.Relay = prev
		return
	}
	s.notifyRelayChanged()
	s.recordAudit(s.actor(r), "relay.config", "", map[string]any{
		"enabled": cur.Enabled, "outbound_tag": cur.EffectiveOutboundTag(),
	})
	s.write(w, map[string]any{"ok": true, "config": toRelayView(cur)})
}

func (s *Server) relaySitesPost(w http.ResponseWriter, r *http.Request) {
	var req stack.RelaySite
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	req.Domain = strings.TrimSpace(req.Domain)
	if req.Domain == "" {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("domain is required"))
		return
	}
	for _, existing := range s.cfg.Relay.Sites {
		if strings.EqualFold(existing.Domain, req.Domain) {
			s.fail(w, http.StatusConflict, fmt.Errorf("site %q already exists", req.Domain))
			return
		}
	}
	prev := s.cfg.Relay.Sites
	s.cfg.Relay.Sites = append(prev, req)
	if !s.save(w) {
		s.cfg.Relay.Sites = prev
		return
	}
	s.notifyRelayChanged()
	s.recordAudit(s.actor(r), "relay.site.add", req.Domain, map[string]any{"enabled": req.Enabled})
	s.write(w, map[string]any{"ok": true, "sites": s.cfg.Relay.Sites})
}

func (s *Server) relaySitesPut(w http.ResponseWriter, r *http.Request) {
	var req stack.RelaySite
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	req.Domain = strings.TrimSpace(req.Domain)
	if req.Domain == "" {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("domain is required"))
		return
	}
	prev := append([]stack.RelaySite(nil), s.cfg.Relay.Sites...)
	found := false
	for i := range s.cfg.Relay.Sites {
		if strings.EqualFold(s.cfg.Relay.Sites[i].Domain, req.Domain) {
			s.cfg.Relay.Sites[i] = req
			found = true
			break
		}
	}
	if !found {
		s.fail(w, http.StatusNotFound, fmt.Errorf("site %q not found", req.Domain))
		return
	}
	if !s.save(w) {
		s.cfg.Relay.Sites = prev
		return
	}
	s.notifyRelayChanged()
	s.recordAudit(s.actor(r), "relay.site.update", req.Domain, map[string]any{"enabled": req.Enabled})
	s.write(w, map[string]any{"ok": true, "sites": s.cfg.Relay.Sites})
}

func (s *Server) relaySitesDelete(w http.ResponseWriter, r *http.Request) {
	domain := strings.TrimSpace(r.URL.Query().Get("domain"))
	if domain == "" {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("domain is required"))
		return
	}
	prev := append([]stack.RelaySite(nil), s.cfg.Relay.Sites...)
	out := s.cfg.Relay.Sites[:0]
	removed := false
	for _, x := range s.cfg.Relay.Sites {
		if strings.EqualFold(x.Domain, domain) {
			removed = true
			continue
		}
		out = append(out, x)
	}
	if !removed {
		s.cfg.Relay.Sites = prev
		s.fail(w, http.StatusNotFound, fmt.Errorf("site %q not found", domain))
		return
	}
	s.cfg.Relay.Sites = out
	if !s.save(w) {
		s.cfg.Relay.Sites = prev
		return
	}
	s.notifyRelayChanged()
	s.recordAudit(s.actor(r), "relay.site.delete", domain, nil)
	s.write(w, map[string]any{"ok": true, "sites": s.cfg.Relay.Sites})
}

func (s *Server) relayStatus(w http.ResponseWriter, r *http.Request) {
	var st relay.Status
	if s.relayStore != nil {
		st = s.relayStore.Snapshot()
	} else {
		// No supervisor running — return a "config-only" view derived from
		// stack.json so the panel can still show what's configured.
		_, _, statePath, _, _ := relay.EffectivePaths(s.cfg.Relay)
		if loaded, err := relay.LoadStatus(statePath); err == nil {
			st = loaded
		}
		st.Enabled = s.cfg.Relay.Enabled
		st.Listen = s.cfg.Relay.EffectiveListen()
		st.OutboundTag = s.cfg.Relay.EffectiveOutboundTag()
		st.EnabledSites = len(s.cfg.Relay.EnabledSites())
		st.TotalSites = len(s.cfg.Relay.Sites)
	}
	s.write(w, map[string]any{"ok": true, "status": st, "managed": s.relaySupervisor != nil})
}

func (s *Server) relayTest(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	result := relay.Probe(ctx, s.cfg.Relay, 10*time.Second)
	s.recordAudit(s.actor(r), "relay.test", "", map[string]any{"ok": result.OK, "latency_ms": result.LatencyMS})
	s.write(w, map[string]any{"ok": result.OK, "probe": result})
}

func (s *Server) relayRestart(w http.ResponseWriter, r *http.Request) {
	if s.relaySupervisor == nil {
		s.fail(w, http.StatusServiceUnavailable, fmt.Errorf("relay supervisor is not running (start xray-stackd with -manage-relay)"))
		return
	}
	s.relaySupervisor.Restart()
	s.recordAudit(s.actor(r), "relay.restart", "", nil)
	s.write(w, map[string]any{"ok": true})
}

func (s *Server) relayLogs(w http.ResponseWriter, r *http.Request) {
	lines := 200
	if v := r.URL.Query().Get("lines"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 2000 {
			lines = n
		}
	}
	_, _, _, logPath, _ := relay.EffectivePaths(s.cfg.Relay)
	out, err := relay.Tail(logPath, lines)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	var events []relay.LogEntry
	if s.relayStore != nil {
		events = s.relayStore.Logs()
	}
	s.write(w, map[string]any{"ok": true, "lines": out, "events": events, "path": logPath})
}

func (s *Server) notifyRelayChanged() {
	if s.relaySupervisor != nil {
		s.relaySupervisor.Reload()
	}
}

func dedupTrim(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
