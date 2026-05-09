package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
	"github.com/sakhtar/xray-stack-zeroone/internal/tunnel"
	"github.com/sakhtar/xray-stack-zeroone/internal/xray"
)

type Server struct{ cfg stack.Config }

func NewServer(cfg stack.Config) http.Handler {
	s := &Server{cfg: cfg}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/config/summary", s.summary)
	mux.HandleFunc("GET /api/xray/generated", s.generatedXray)
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

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	checks := tunnel.CheckAll(r.Context(), s.cfg.Tunnels, s.cfg.Failover.ProbeIP, s.cfg.Failover.ProbePort)
	s.write(w, map[string]any{"ok": true, "generated_at": time.Now().Format(time.RFC3339), "tunnels": checks})
}

func (s *Server) summary(w http.ResponseWriter, r *http.Request) {
	s.write(w, map[string]any{"public_ip": s.cfg.Server.PublicIP, "users": len(s.cfg.Xray.Users), "socks": len(s.cfg.Xray.Inbounds.PublicSOCKS), "tunnels": s.cfg.Tunnels, "failover": s.cfg.Failover})
}

func (s *Server) generatedXray(w http.ResponseWriter, r *http.Request) {
	s.write(w, xray.Generate(s.cfg))
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8"><title>Xray Stack</title></head><body><h1>Xray Stack</h1><p>Go control plane is running.</p><script>fetch('/api/config/summary').then(r=>r.json()).then(d=>document.body.appendChild(document.createElement('pre')).textContent=JSON.stringify(d,null,2))</script></body></html>`))
}
