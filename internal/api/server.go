package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/bandwidth"
	"github.com/sakhtar/xray-stack-zeroone/internal/enforce"
	"github.com/sakhtar/xray-stack-zeroone/internal/failover"
	"github.com/sakhtar/xray-stack-zeroone/internal/links"
	"github.com/sakhtar/xray-stack-zeroone/internal/monitor"
	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
	"github.com/sakhtar/xray-stack-zeroone/internal/tunnel"
	"github.com/sakhtar/xray-stack-zeroone/internal/usage"
	"github.com/sakhtar/xray-stack-zeroone/internal/xray"
)

type Server struct {
	cfg        stack.Config
	configPath string
	allowApply bool
}

func NewServer(cfg stack.Config, configPath string, allowApply bool) http.Handler {
	s := &Server{cfg: cfg, configPath: configPath, allowApply: allowApply}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("POST /api/test/connect", s.testConnect)
	mux.HandleFunc("GET /api/system", s.system)
	mux.HandleFunc("GET /api/users/activity", s.userActivity)
	mux.HandleFunc("GET /api/config/summary", s.summary)
	mux.HandleFunc("GET /api/xray/generated", s.generatedXray)
	mux.HandleFunc("GET /api/xray/apply-plan", s.xrayApplyPlan)
	mux.HandleFunc("POST /api/xray/apply", s.xrayApply)
	mux.HandleFunc("GET /api/failover/decision", s.failoverDecision)
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
	mux.HandleFunc("POST /api/direct-domains", s.addDirectDomain)
	mux.HandleFunc("DELETE /api/direct-domains", s.deleteDirectDomain)
	mux.HandleFunc("POST /api/socks", s.addSOCKS)
	mux.HandleFunc("PUT /api/socks", s.updateSOCKS)
	mux.HandleFunc("DELETE /api/socks", s.deleteSOCKS)
	mux.HandleFunc("GET /", s.ui)
	return mux
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

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	checks := tunnel.CheckAll(r.Context(), s.cfg.Tunnels, s.cfg.Failover.ProbeTargets())
	s.write(w, map[string]any{"ok": true, "generated_at": time.Now().Format(time.RFC3339), "tunnels": checks})
}

func (s *Server) system(w http.ResponseWriter, r *http.Request) {
	s.write(w, monitor.SystemSnapshot(s.cfg))
}

func (s *Server) testConnect(w http.ResponseWriter, r *http.Request) {
	var req tunnel.ConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, 400, err)
		return
	}
	switch req.Route {
	case "", "direct":
		req.Route = "direct"
	case "priority-upstream":
		if s.cfg.Xray.Outbounds.Fallback.Address == "" || s.cfg.Xray.Outbounds.Fallback.Port == 0 {
			s.fail(w, 400, fmt.Errorf("fallback outbound is not configured"))
			return
		}
		req.Target = s.cfg.Xray.Outbounds.Fallback.Address
		req.Port = s.cfg.Xray.Outbounds.Fallback.Port
	case "tun0", "tun1":
		req.Interface = req.Route
	default:
		found := false
		for _, t := range s.cfg.Tunnels {
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

func (s *Server) failoverDecision(w http.ResponseWriter, r *http.Request) {
	checks := tunnel.CheckAll(r.Context(), s.cfg.Tunnels, s.cfg.Failover.ProbeTargets())
	state, err := failover.LoadState(s.cfg.Server.FailoverStatePath)
	if err != nil {
		s.fail(w, 500, err)
		return
	}
	decision, nextState := failover.Decide(s.cfg, state, checks, time.Now())
	s.write(w, map[string]any{"checks": checks, "decision": decision, "state": state, "next_state": nextState})
}

func (s *Server) summary(w http.ResponseWriter, r *http.Request) {
	userViews := make([]map[string]any, 0, len(s.cfg.Xray.Users))
	for _, u := range s.cfg.Xray.Users {
		userViews = append(userViews, map[string]any{
			"email":          u.Email,
			"uuid":           u.UUID,
			"enabled":        u.Enabled,
			"banned_until":   u.BannedUntil,
			"quota_bytes":    u.QuotaBytes,
			"download_mbps":  u.DownloadMbps,
			"upload_mbps":    u.UploadMbps,
			"bandwidth_port": u.BandwidthPort,
			"links":          links.VLESS(s.cfg, u),
		})
	}
	socksViews := make([]map[string]any, 0, len(s.cfg.Xray.Inbounds.PublicSOCKS))
	for _, socks := range s.cfg.Xray.Inbounds.PublicSOCKS {
		socksViews = append(socksViews, map[string]any{
			"name":     socks.Name,
			"listen":   socks.Listen,
			"port":     socks.Port,
			"username": socks.Username,
			"password": socks.Password,
			"links":    links.SOCKS(s.cfg, socks),
		})
	}
	s.write(w, map[string]any{
		"public_ip":      s.cfg.Server.PublicIP,
		"users":          len(s.cfg.Xray.Users),
		"socks":          len(s.cfg.Xray.Inbounds.PublicSOCKS),
		"user_items":     userViews,
		"socks_items":    socksViews,
		"direct_domains": s.cfg.Xray.Routing.DirectDomains,
		"block_domains":  s.cfg.Xray.Routing.BlockDomains,
		"manual_blocks":  s.cfg.Xray.Routing.ManualBlockDomains,
		"tunnels":        s.cfg.Tunnels,
		"failover":       s.cfg.Failover,
		"allow_apply":    s.allowApply,
	})
}

func (s *Server) generatedXray(w http.ResponseWriter, r *http.Request) {
	s.write(w, xray.Generate(s.cfg))
}

func (s *Server) xrayApplyPlan(w http.ResponseWriter, r *http.Request) {
	m := xray.Manager{}
	plan, _, err := m.Plan(r.Context(), s.cfg)
	if err != nil {
		s.fail(w, 400, err)
		return
	}
	s.write(w, map[string]any{"ok": true, "valid": true, "config_path": s.cfg.Server.XrayConfigPath, "allow_apply": s.allowApply, "changed": plan.Changed, "backup_path": plan.BackupPath})
}

func (s *Server) xrayApply(w http.ResponseWriter, r *http.Request) {
	if !s.allowApply {
		s.fail(w, http.StatusForbidden, fmt.Errorf("apply is disabled; start with -allow-apply"))
		return
	}
	plan, err := (xray.Manager{}).Apply(r.Context(), s.cfg)
	if err != nil {
		s.fail(w, 500, err)
		return
	}
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
	s.write(w, map[string]any{"ok": true, "bandwidth_port": port})
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
	s.write(w, map[string]any{"ok": true, "domain": req.Domain})
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
	s.write(w, map[string]any{"ok": true})
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
	s.write(w, map[string]any{"ok": true})
}

func (s *Server) ui(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Server.UIPath != "" && fileExists(filepath.Join(s.cfg.Server.UIPath, "index.html")) {
		s.serveUI(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8"><title>Xray Stack</title></head><body><h1>Xray Stack</h1><p>Go control plane is running.</p><script>fetch('/api/config/summary').then(r=>r.json()).then(d=>document.body.appendChild(document.createElement('pre')).textContent=JSON.stringify(d,null,2))</script></body></html>`))
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
