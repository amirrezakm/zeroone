package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/sakhtar/xray-stack-zeroone/internal/usage"
)

func (s *Server) setUserPeriods(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email             string `json:"email"`
		DailyQuotaBytes   int64  `json:"daily_quota_bytes"`
		WeeklyQuotaBytes  int64  `json:"weekly_quota_bytes"`
		MonthlyQuotaBytes int64  `json:"monthly_quota_bytes"`
		DailyResetHHMM    string `json:"daily_reset_hhmm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	if req.Email == "" {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("email is required"))
		return
	}
	// Capture the prior HHMM so we know whether to invalidate the cached
	// next-reset deadline. Changing the hh:mm should make the displayed
	// "next reset" reflect the new time without waiting for the existing
	// deadline to elapse first.
	priorHHMM := ""
	for _, u := range s.cfg.Xray.Users {
		if u.Email == req.Email {
			priorHHMM = u.EffectiveDailyResetHHMM()
			break
		}
	}
	if err := s.cfg.SetUserPeriodQuotas(req.Email, req.DailyQuotaBytes, req.WeeklyQuotaBytes, req.MonthlyQuotaBytes, req.DailyResetHHMM); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	if !s.save(w) {
		return
	}
	if newHHMM := req.DailyResetHHMM; newHHMM != "" && newHHMM != priorHHMM && s.cfg.Server.UserUsagePath != "" {
		if state, err := usage.LoadUserState(s.cfg.Server.UserUsagePath); err == nil {
			if state.Periods == nil {
				state.Periods = map[string]usage.PeriodMeta{}
			}
			meta := state.Periods[req.Email]
			meta.DailyResetAt = 0
			state.Periods[req.Email] = meta
			_ = usage.SaveUserState(s.cfg.Server.UserUsagePath, state)
		}
	}
	s.recordAudit(s.actor(r), "user.periods", req.Email, map[string]any{
		"daily_quota_bytes":   req.DailyQuotaBytes,
		"weekly_quota_bytes":  req.WeeklyQuotaBytes,
		"monthly_quota_bytes": req.MonthlyQuotaBytes,
		"daily_reset_hhmm":    req.DailyResetHHMM,
	})
	s.write(w, map[string]any{"ok": true})
}

func (s *Server) setUserMaxSessions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email       string `json:"email"`
		MaxSessions int    `json:"max_sessions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	if req.Email == "" {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("email is required"))
		return
	}
	if err := s.cfg.SetUserMaxSessions(req.Email, req.MaxSessions); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	if !s.save(w) {
		return
	}
	s.recordAudit(s.actor(r), "user.max_sessions", req.Email, map[string]any{"max_sessions": req.MaxSessions})
	s.write(w, map[string]any{"ok": true})
}
