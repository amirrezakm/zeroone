package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/bandwidth"
	"github.com/sakhtar/xray-stack-zeroone/internal/enforce"
	"github.com/sakhtar/xray-stack-zeroone/internal/failover"
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
	mux.HandleFunc("DELETE /api/users", s.deleteUser)
	mux.HandleFunc("POST /api/users/quota", s.setUserQuota)
	mux.HandleFunc("POST /api/users/bandwidth", s.setUserBandwidth)
	mux.HandleFunc("POST /api/direct-domains", s.addDirectDomain)
	mux.HandleFunc("DELETE /api/direct-domains", s.deleteDirectDomain)
	mux.HandleFunc("POST /api/socks", s.addSOCKS)
	mux.HandleFunc("GET /", s.index)
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
	checks := tunnel.CheckAll(r.Context(), s.cfg.Tunnels, s.cfg.Failover.ProbeIP, s.cfg.Failover.ProbePort)
	s.write(w, map[string]any{"ok": true, "generated_at": time.Now().Format(time.RFC3339), "tunnels": checks})
}

func (s *Server) failoverDecision(w http.ResponseWriter, r *http.Request) {
	checks := tunnel.CheckAll(r.Context(), s.cfg.Tunnels, s.cfg.Failover.ProbeIP, s.cfg.Failover.ProbePort)
	decision, _ := failover.Decide(s.cfg, failover.State{}, checks, time.Now())
	s.write(w, map[string]any{"checks": checks, "decision": decision})
}

func (s *Server) summary(w http.ResponseWriter, r *http.Request) {
	s.write(w, map[string]any{"public_ip": s.cfg.Server.PublicIP, "users": len(s.cfg.Xray.Users), "socks": len(s.cfg.Xray.Inbounds.PublicSOCKS), "tunnels": s.cfg.Tunnels, "failover": s.cfg.Failover, "allow_apply": s.allowApply})
}

func (s *Server) generatedXray(w http.ResponseWriter, r *http.Request) {
	s.write(w, xray.Generate(s.cfg))
}

func (s *Server) xrayApplyPlan(w http.ResponseWriter, r *http.Request) {
	m := xray.Manager{}
	rendered, err := m.Render(s.cfg)
	if err != nil {
		s.fail(w, 500, err)
		return
	}
	if err := m.Validate(r.Context(), s.cfg, rendered); err != nil {
		s.fail(w, 400, err)
		return
	}
	s.write(w, map[string]any{"ok": true, "valid": true, "config_path": s.cfg.Server.XrayConfigPath, "allow_apply": s.allowApply})
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
	s.write(w, map[string]any{"ok": true, "email": req.Email})
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

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8"><title>Xray Stack</title></head><body><h1>Xray Stack</h1><p>Go control plane is running.</p><script>fetch('/api/config/summary').then(r=>r.json()).then(d=>document.body.appendChild(document.createElement('pre')).textContent=JSON.stringify(d,null,2))</script></body></html>`))
}

func (s *Server) xrayAPIServer() string {
	port := s.cfg.Xray.APIPort
	if port == 0 {
		port = 10085
	}
	return fmt.Sprintf("127.0.0.1:%d", port)
}
