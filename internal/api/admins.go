package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/amirrezakm/zeroone/internal/auth"
	"github.com/amirrezakm/zeroone/internal/stack"
)

// ensureSessionSecret persists a session-signing secret in the panel
// config if one isn't already set. Called lazily on first login attempt
// so the operator never has to seed it manually.
func (s *Server) ensureSessionSecret() (string, error) {
	if s.cfg.Panel.SessionSecret != "" {
		return s.cfg.Panel.SessionSecret, nil
	}
	secret, err := auth.NewSessionSecret()
	if err != nil {
		return "", err
	}
	s.cfg.Panel.SessionSecret = secret
	if err := stack.Save(s.configPath, s.cfg); err != nil {
		return "", err
	}
	return secret, nil
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("username and password are required"))
		return
	}
	// Refresh from disk so a concurrent admin add/remove is visible. We
	// assign to s.cfg here and then mutate s.cfg in place — taking a local
	// copy and writing s.cfg = local later would clobber any field
	// ensureSessionSecret had just persisted (such as session_secret).
	fresh, ok := s.currentConfig(w)
	if !ok {
		return
	}
	s.cfg = fresh
	idx := -1
	for i := range s.cfg.Panel.Admins {
		if s.cfg.Panel.Admins[i].Username == req.Username {
			idx = i
			break
		}
	}
	if idx < 0 || !auth.VerifyPassword(req.Password, s.cfg.Panel.Admins[idx].PasswordHash) {
		// Constant-message reply so callers can't probe for valid usernames.
		s.fail(w, http.StatusUnauthorized, fmt.Errorf("invalid username or password"))
		return
	}
	secret, err := s.ensureSessionSecret()
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	username := s.cfg.Panel.Admins[idx].Username
	token, expires, err := auth.IssueSession(secret, username)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.cfg.Panel.Admins[idx].LastLogin = time.Now().Unix()
	_ = stack.Save(s.configPath, s.cfg)
	auth.SetSessionCookie(w, r, token, expires)
	s.recordAudit(username, "admin.login", username, nil)
	s.write(w, map[string]any{
		"ok":         true,
		"username":   username,
		"expires_at": expires.Unix(),
	})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	user := auth.SessionFromRequest(r, s.cfg.Panel.SessionSecret)
	auth.ClearSessionCookie(w, r)
	if user != "" {
		s.recordAudit(user, "admin.logout", user, nil)
	}
	s.write(w, map[string]any{"ok": true})
}

// me returns whoever the caller authenticated as: the session username
// when a session cookie is present, "token:<id>" when a Bearer token was
// used, or "" when authentication is open (bootstrap, no admins yet).
//
// Reads admins/session-secret fresh from disk so the React auth gate
// flips from bootstrap to login the instant the installer's
// `zeroone admin add` finishes — no daemon restart required.
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	fresh, ok := s.currentConfig(w)
	if !ok {
		return
	}
	username := auth.SessionFromRequest(r, fresh.Panel.SessionSecret)
	authKind := ""
	if username != "" {
		authKind = "session"
	} else if r.Header.Get("Authorization") != "" {
		authKind = "token"
	}
	s.write(w, map[string]any{
		"ok":               true,
		"username":         username,
		"auth":             authKind,
		"admins_count":     len(fresh.Panel.Admins),
		"bootstrap_needed": len(fresh.Panel.Admins) == 0,
	})
}

func (s *Server) adminsList(w http.ResponseWriter, r *http.Request) {
	items := make([]map[string]any, 0, len(s.cfg.Panel.Admins))
	for _, a := range s.cfg.Panel.Admins {
		items = append(items, map[string]any{
			"username":   a.Username,
			"created_at": a.CreatedAt,
			"last_login": a.LastLogin,
		})
	}
	s.write(w, map[string]any{"ok": true, "admins": items})
}

func (s *Server) adminCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	if req.Username == "" || req.Password == "" {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("username and password are required"))
		return
	}
	if len(req.Password) < 8 {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("password must be at least 8 characters"))
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.cfg.AddAdmin(req.Username, hash); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	// Stamp CreatedAt on the newly-added admin (AddAdmin keeps zero so
	// callers can override; we always want a real timestamp from the API).
	for i := range s.cfg.Panel.Admins {
		if s.cfg.Panel.Admins[i].Username == req.Username {
			s.cfg.Panel.Admins[i].CreatedAt = time.Now().Unix()
			break
		}
	}
	if !s.save(w) {
		return
	}
	s.recordAudit(s.actor(r), "admin.create", req.Username, nil)
	s.write(w, map[string]any{"ok": true, "username": req.Username})
}

func (s *Server) adminSetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	if req.Username == "" || req.Password == "" {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("username and password are required"))
		return
	}
	if len(req.Password) < 8 {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("password must be at least 8 characters"))
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.cfg.SetAdminPassword(req.Username, hash); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	if !s.save(w) {
		return
	}
	s.recordAudit(s.actor(r), "admin.password", req.Username, nil)
	s.write(w, map[string]any{"ok": true})
}

func (s *Server) adminDelete(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	if err := s.cfg.DeleteAdmin(username); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	if !s.save(w) {
		return
	}
	s.recordAudit(s.actor(r), "admin.delete", username, nil)
	s.write(w, map[string]any{"ok": true})
}
