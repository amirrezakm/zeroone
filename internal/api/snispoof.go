package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/amirrezakm/zeroone/internal/snispoof"
	"github.com/amirrezakm/zeroone/internal/stack"
)

// snispoofView is the panel-facing config representation.
type snispoofView struct {
	Enabled      bool              `json:"enabled"`
	FakeDomain   string            `json:"fake_domain"`
	Method       string            `json:"method"`
	FakeTTL      int               `json:"fake_ttl"`
	Listen       string            `json:"listen"`
	Strategy     string            `json:"strategy"`
	ExtraArgs    []string          `json:"extra_args"`
	OutboundTag  string            `json:"outbound_tag"`
	Sites        []stack.RelaySite `json:"sites"`
	InboundTags  []string          `json:"inbound_tags"`
	TunName      string            `json:"tun_name"`
	TunAddr      string            `json:"tun_addr"`
	FirewallMark int               `json:"firewall_mark"`
	RouteTable   int               `json:"route_table"`
	MTU          int               `json:"mtu"`
	Binary       string            `json:"binary"`
	Tun2socks    string            `json:"tun2socks_binary"`
	HealthProbe  string            `json:"health_probe"`
	Notes        string            `json:"notes"`
	Methods      []string          `json:"methods"`
	Defaults     snispoofDefaults  `json:"defaults"`
}

type snispoofDefaults struct {
	Listen       string `json:"listen"`
	OutboundTag  string `json:"outbound_tag"`
	Method       string `json:"method"`
	FakeDomain   string `json:"fake_domain"`
	FakeTTL      int    `json:"fake_ttl"`
	TunName      string `json:"tun_name"`
	TunAddr      string `json:"tun_addr"`
	FirewallMark int    `json:"firewall_mark"`
	RouteTable   int    `json:"route_table"`
	MTU          int    `json:"mtu"`
	Binary       string `json:"binary"`
	Tun2socks    string `json:"tun2socks_binary"`
	HealthProbe  string `json:"health_probe"`
}

func toSNISpoofView(c stack.SNISpoofConfig) snispoofView {
	sites := append([]stack.RelaySite(nil), c.Sites...)
	if sites == nil {
		sites = []stack.RelaySite{}
	}
	return snispoofView{
		Enabled:      c.Enabled,
		FakeDomain:   c.FakeDomain,
		Method:       c.Method,
		FakeTTL:      c.FakeTTL,
		Listen:       c.Listen,
		Strategy:     c.Strategy,
		ExtraArgs:    append([]string{}, c.ExtraArgs...),
		OutboundTag:  c.OutboundTag,
		Sites:        sites,
		InboundTags:  append([]string{}, c.InboundTags...),
		TunName:      c.TunName,
		TunAddr:      c.TunAddr,
		FirewallMark: c.FirewallMark,
		RouteTable:   c.RouteTable,
		MTU:          c.MTU,
		Binary:       c.Binary,
		Tun2socks:    c.Tun2socksBinary,
		HealthProbe:  c.HealthProbe,
		Notes:        c.Notes,
		Methods:      stack.SNISpoofMethods,
		Defaults: snispoofDefaults{
			Listen:       stack.DefaultSNISpoofListen,
			OutboundTag:  stack.DefaultSNISpoofOutboundTag,
			Method:       stack.DefaultSNISpoofMethod,
			FakeDomain:   stack.DefaultSNISpoofFakeDomain,
			FakeTTL:      stack.DefaultSNISpoofFakeTTL,
			TunName:      stack.DefaultSNISpoofTunName,
			TunAddr:      stack.DefaultSNISpoofTunAddr,
			FirewallMark: stack.DefaultSNISpoofFirewallMark,
			RouteTable:   stack.DefaultSNISpoofRouteTable,
			MTU:          stack.DefaultSNISpoofMTU,
			Binary:       stack.DefaultSNISpoofBinary,
			Tun2socks:    stack.DefaultSNISpoofTun2socks,
			HealthProbe:  stack.DefaultSNISpoofHealthProbe,
		},
	}
}

func (s *Server) snispoofConfigGet(w http.ResponseWriter, r *http.Request) {
	s.write(w, map[string]any{"ok": true, "config": toSNISpoofView(s.cfg.SNISpoof)})
}

// snispoofConfigPut accepts a partial update; omitted fields keep existing values.
func (s *Server) snispoofConfigPut(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled      *bool     `json:"enabled,omitempty"`
		FakeDomain   *string   `json:"fake_domain,omitempty"`
		Method       *string   `json:"method,omitempty"`
		FakeTTL      *int      `json:"fake_ttl,omitempty"`
		Listen       *string   `json:"listen,omitempty"`
		Strategy     *string   `json:"strategy,omitempty"`
		ExtraArgs    *[]string `json:"extra_args,omitempty"`
		OutboundTag  *string   `json:"outbound_tag,omitempty"`
		InboundTags  *[]string `json:"inbound_tags,omitempty"`
		TunName      *string   `json:"tun_name,omitempty"`
		TunAddr      *string   `json:"tun_addr,omitempty"`
		FirewallMark *int      `json:"firewall_mark,omitempty"`
		RouteTable   *int      `json:"route_table,omitempty"`
		MTU          *int      `json:"mtu,omitempty"`
		Binary       *string   `json:"binary,omitempty"`
		Tun2socks    *string   `json:"tun2socks_binary,omitempty"`
		HealthProbe  *string   `json:"health_probe,omitempty"`
		Notes        *string   `json:"notes,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	cur := s.cfg.SNISpoof
	if req.Enabled != nil {
		cur.Enabled = *req.Enabled
	}
	if req.FakeDomain != nil {
		cur.FakeDomain = strings.TrimSpace(*req.FakeDomain)
	}
	if req.Method != nil {
		cur.Method = strings.TrimSpace(*req.Method)
	}
	if req.FakeTTL != nil {
		cur.FakeTTL = *req.FakeTTL
	}
	if req.Listen != nil {
		cur.Listen = strings.TrimSpace(*req.Listen)
	}
	if req.Strategy != nil {
		cur.Strategy = strings.TrimSpace(*req.Strategy)
	}
	if req.ExtraArgs != nil {
		cur.ExtraArgs = trimSlice(*req.ExtraArgs)
	}
	if req.OutboundTag != nil {
		cur.OutboundTag = strings.TrimSpace(*req.OutboundTag)
	}
	if req.InboundTags != nil {
		cur.InboundTags = dedupTrim(*req.InboundTags)
	}
	if req.TunName != nil {
		cur.TunName = strings.TrimSpace(*req.TunName)
	}
	if req.TunAddr != nil {
		cur.TunAddr = strings.TrimSpace(*req.TunAddr)
	}
	if req.FirewallMark != nil {
		cur.FirewallMark = *req.FirewallMark
	}
	if req.RouteTable != nil {
		cur.RouteTable = *req.RouteTable
	}
	if req.MTU != nil {
		cur.MTU = *req.MTU
	}
	if req.Binary != nil {
		cur.Binary = strings.TrimSpace(*req.Binary)
	}
	if req.Tun2socks != nil {
		cur.Tun2socksBinary = strings.TrimSpace(*req.Tun2socks)
	}
	if req.HealthProbe != nil {
		cur.HealthProbe = strings.TrimSpace(*req.HealthProbe)
	}
	if req.Notes != nil {
		cur.Notes = *req.Notes
	}
	prev := s.cfg.SNISpoof
	s.cfg.SNISpoof = cur
	if !s.save(w) {
		s.cfg.SNISpoof = prev
		return
	}
	s.notifySNISpoofChanged()
	s.recordAudit(s.actor(r), "snispoof.config", "", map[string]any{
		"enabled": cur.Enabled, "method": cur.EffectiveMethod(), "fake_domain": cur.EffectiveFakeDomain(),
	})
	s.write(w, map[string]any{"ok": true, "config": toSNISpoofView(cur)})
}

func (s *Server) snispoofSitesPost(w http.ResponseWriter, r *http.Request) {
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
	for _, existing := range s.cfg.SNISpoof.Sites {
		if strings.EqualFold(existing.Domain, req.Domain) {
			s.fail(w, http.StatusConflict, fmt.Errorf("site %q already exists", req.Domain))
			return
		}
	}
	prev := s.cfg.SNISpoof.Sites
	s.cfg.SNISpoof.Sites = append(prev, req)
	if !s.save(w) {
		s.cfg.SNISpoof.Sites = prev
		return
	}
	s.notifySNISpoofChanged()
	s.recordAudit(s.actor(r), "snispoof.site.add", req.Domain, map[string]any{"enabled": req.Enabled})
	s.write(w, map[string]any{"ok": true, "sites": s.cfg.SNISpoof.Sites})
}

func (s *Server) snispoofSitesPut(w http.ResponseWriter, r *http.Request) {
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
	prev := append([]stack.RelaySite(nil), s.cfg.SNISpoof.Sites...)
	found := false
	for i := range s.cfg.SNISpoof.Sites {
		if strings.EqualFold(s.cfg.SNISpoof.Sites[i].Domain, req.Domain) {
			s.cfg.SNISpoof.Sites[i] = req
			found = true
			break
		}
	}
	if !found {
		s.fail(w, http.StatusNotFound, fmt.Errorf("site %q not found", req.Domain))
		return
	}
	if !s.save(w) {
		s.cfg.SNISpoof.Sites = prev
		return
	}
	s.notifySNISpoofChanged()
	s.recordAudit(s.actor(r), "snispoof.site.update", req.Domain, map[string]any{"enabled": req.Enabled})
	s.write(w, map[string]any{"ok": true, "sites": s.cfg.SNISpoof.Sites})
}

func (s *Server) snispoofSitesDelete(w http.ResponseWriter, r *http.Request) {
	domain := strings.TrimSpace(r.URL.Query().Get("domain"))
	if domain == "" {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("domain is required"))
		return
	}
	prev := append([]stack.RelaySite(nil), s.cfg.SNISpoof.Sites...)
	out := s.cfg.SNISpoof.Sites[:0]
	removed := false
	for _, x := range s.cfg.SNISpoof.Sites {
		if strings.EqualFold(x.Domain, domain) {
			removed = true
			continue
		}
		out = append(out, x)
	}
	if !removed {
		s.cfg.SNISpoof.Sites = prev
		s.fail(w, http.StatusNotFound, fmt.Errorf("site %q not found", domain))
		return
	}
	s.cfg.SNISpoof.Sites = out
	if !s.save(w) {
		s.cfg.SNISpoof.Sites = prev
		return
	}
	s.notifySNISpoofChanged()
	s.recordAudit(s.actor(r), "snispoof.site.delete", domain, nil)
	s.write(w, map[string]any{"ok": true, "sites": s.cfg.SNISpoof.Sites})
}

func (s *Server) snispoofStatus(w http.ResponseWriter, r *http.Request) {
	var st snispoof.Status
	if s.snispoofStore != nil {
		st = s.snispoofStore.Snapshot()
	} else {
		_, _, _, statePath, _, _ := snispoof.EffectivePaths(s.cfg.SNISpoof)
		if loaded, err := snispoof.LoadStatus(statePath); err == nil {
			st = loaded
		}
		st.Enabled = s.cfg.SNISpoof.Enabled
		st.Listen = s.cfg.SNISpoof.EffectiveListen()
		st.OutboundTag = s.cfg.SNISpoof.EffectiveOutboundTag()
		st.Method = s.cfg.SNISpoof.EffectiveMethod()
		st.FakeDomain = s.cfg.SNISpoof.EffectiveFakeDomain()
		st.TunName = s.cfg.SNISpoof.EffectiveTunName()
		st.EnabledSites = len(s.cfg.SNISpoof.EnabledSites())
		st.TotalSites = len(s.cfg.SNISpoof.Sites)
	}
	s.write(w, map[string]any{"ok": true, "status": st, "managed": s.snispoofSupervisor != nil})
}

func (s *Server) snispoofTest(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	result := snispoof.Probe(ctx, s.cfg.SNISpoof, 10*time.Second)
	s.recordAudit(s.actor(r), "snispoof.test", "", map[string]any{"ok": result.OK, "latency_ms": result.LatencyMS})
	s.write(w, map[string]any{"ok": result.OK, "probe": result})
}

func (s *Server) snispoofRestart(w http.ResponseWriter, r *http.Request) {
	if s.snispoofSupervisor == nil {
		s.fail(w, http.StatusServiceUnavailable, fmt.Errorf("sni-spoof supervisor is not running (start zeroone with -manage-snispoof)"))
		return
	}
	s.snispoofSupervisor.Restart()
	s.recordAudit(s.actor(r), "snispoof.restart", "", nil)
	s.write(w, map[string]any{"ok": true})
}

func (s *Server) snispoofLogs(w http.ResponseWriter, r *http.Request) {
	lines := 200
	if v := r.URL.Query().Get("lines"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 2000 {
			lines = n
		}
	}
	_, _, _, _, logPath, _ := snispoof.EffectivePaths(s.cfg.SNISpoof)
	out, err := snispoof.Tail(logPath, lines)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	var events []snispoof.LogEntry
	if s.snispoofStore != nil {
		events = s.snispoofStore.Logs()
	}
	s.write(w, map[string]any{"ok": true, "lines": out, "events": events, "path": logPath})
}

func (s *Server) notifySNISpoofChanged() {
	if s.snispoofSupervisor != nil {
		s.snispoofSupervisor.Reload()
	}
}

func trimSlice(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}
