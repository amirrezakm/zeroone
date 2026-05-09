package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/failover"
	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
	"github.com/sakhtar/xray-stack-zeroone/internal/tunnel"
	"github.com/sakhtar/xray-stack-zeroone/internal/xray"
)

type Server struct {
	cfg        stack.Config
	configPath string
}

func NewServer(cfg stack.Config, configPath string) http.Handler {
	s := &Server{cfg: cfg, configPath: configPath}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/config/summary", s.summary)
	mux.HandleFunc("GET /api/xray/generated", s.generatedXray)
	mux.HandleFunc("GET /api/failover/decision", s.failoverDecision)
	mux.HandleFunc("POST /api/users", s.addUser)
	mux.HandleFunc("DELETE /api/users", s.deleteUser)
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
	s.write(w, map[string]any{"public_ip": s.cfg.Server.PublicIP, "users": len(s.cfg.Xray.Users), "socks": len(s.cfg.Xray.Inbounds.PublicSOCKS), "tunnels": s.cfg.Tunnels, "failover": s.cfg.Failover})
}

func (s *Server) generatedXray(w http.ResponseWriter, r *http.Request) {
	s.write(w, xray.Generate(s.cfg))
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
