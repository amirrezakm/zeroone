package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/auth"
	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
)

func (s *Server) tokensList(w http.ResponseWriter, r *http.Request) {
	out := make([]map[string]any, 0, len(s.cfg.Panel.Tokens))
	for _, t := range s.cfg.Panel.Tokens {
		out = append(out, map[string]any{
			"id":         t.ID,
			"scope":      t.Scope,
			"created_at": t.CreatedAt,
			"last_used":  t.LastUsed,
			"hash_short": t.Hash[:12],
		})
	}
	s.write(w, map[string]any{"ok": true, "tokens": out})
}

func (s *Server) tokenCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID    string `json:"id"`
		Scope string `json:"scope"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.ID == "" {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("id required"))
		return
	}
	for _, t := range s.cfg.Panel.Tokens {
		if t.ID == req.ID {
			s.fail(w, http.StatusConflict, fmt.Errorf("token id %q already exists", req.ID))
			return
		}
	}
	token, err := auth.Generate()
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	entry := stack.APIToken{
		ID:        req.ID,
		Hash:      auth.Hash(token),
		Scope:     req.Scope,
		CreatedAt: time.Now().Unix(),
	}
	s.cfg.Panel.Tokens = append(s.cfg.Panel.Tokens, entry)
	if !s.save(w) {
		return
	}
	s.recordAudit(s.actor(r), "token.create", req.ID, map[string]any{"scope": req.Scope})
	// The plaintext token is returned exactly once. The hash is what we keep.
	s.write(w, map[string]any{"ok": true, "id": entry.ID, "token": token, "note": "Save this token now — it cannot be retrieved later."})
}

func (s *Server) tokenDelete(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("id query param required"))
		return
	}
	next := s.cfg.Panel.Tokens[:0]
	removed := false
	for _, t := range s.cfg.Panel.Tokens {
		if t.ID == id {
			removed = true
			continue
		}
		next = append(next, t)
	}
	if !removed {
		s.fail(w, http.StatusNotFound, fmt.Errorf("token %q not found", id))
		return
	}
	s.cfg.Panel.Tokens = next
	if !s.save(w) {
		return
	}
	s.recordAudit(s.actor(r), "token.delete", id, nil)
	s.write(w, map[string]any{"ok": true, "id": id})
}
