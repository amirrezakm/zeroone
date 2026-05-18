package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func (s *Server) metricsHandler(w http.ResponseWriter, r *http.Request) {
	if s.metrics == nil {
		s.write(w, map[string]any{"ok": true, "samples": []any{}, "step_seconds": 5})
		return
	}
	rangeStr := r.URL.Query().Get("range")
	now := time.Now().Unix()
	var since int64
	var samples []any
	var stepSeconds int64
	switch rangeStr {
	case "24h":
		since = now - 24*3600
		for _, sample := range s.metrics.Coarse.Snapshot(since) {
			samples = append(samples, sample)
		}
		stepSeconds = int64(s.metrics.Coarse.Step().Seconds())
	default: // 1h
		since = now - 3600
		for _, sample := range s.metrics.Fine.Snapshot(since) {
			samples = append(samples, sample)
		}
		stepSeconds = int64(s.metrics.Fine.Step().Seconds())
	}
	s.write(w, map[string]any{"ok": true, "samples": samples, "step_seconds": stepSeconds, "since": since})
}

func (s *Server) eventsHandler(w http.ResponseWriter, r *http.Request) {
	if s.events == nil {
		s.fail(w, http.StatusServiceUnavailable, fmt.Errorf("events not available"))
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.fail(w, http.StatusInternalServerError, fmt.Errorf("streaming not supported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	ch := s.events.Subscribe()
	defer s.events.Unsubscribe(ch)

	// Initial hello so clients know the stream is live.
	_, _ = w.Write([]byte("event: hello\ndata: {}\n\n"))
	flusher.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			payload, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Kind, payload)
			flusher.Flush()
		case <-heartbeat.C:
			_, _ = w.Write([]byte(": ping\n\n"))
			flusher.Flush()
		}
	}
}

func (s *Server) auditHandler(w http.ResponseWriter, r *http.Request) {
	if s.audit == nil {
		s.write(w, map[string]any{"ok": true, "entries": []any{}})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 200
	}
	entries, err := s.audit.Tail(limit)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.write(w, map[string]any{"ok": true, "entries": entries})
}

func (s *Server) snapshotsList(w http.ResponseWriter, r *http.Request) {
	if s.snapshots == nil {
		s.write(w, map[string]any{"ok": true, "snapshots": []any{}})
		return
	}
	list, err := s.snapshots.List()
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.write(w, map[string]any{"ok": true, "snapshots": list})
}

func (s *Server) snapshotsCreate(w http.ResponseWriter, r *http.Request) {
	if s.snapshots == nil {
		s.fail(w, http.StatusServiceUnavailable, fmt.Errorf("snapshots not available"))
		return
	}
	info, err := s.snapshots.Capture(s.configPath, s.cfg.Server.XrayConfigPath)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.recordAudit(s.actor(r), "snapshot.create", info.ID, nil)
	s.write(w, map[string]any{"ok": true, "snapshot": info})
}

// xrayLogs returns recent journalctl output for the xray service. Used by the
// panel's Live logs view. Lines parameter is clamped to keep responses small.
func (s *Server) xrayLogs(w http.ResponseWriter, r *http.Request) {
	lines := 200
	if v := r.URL.Query().Get("lines"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 2000 {
			lines = n
		}
	}
	unit := "xray"
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "journalctl", "-u", unit, "-n", strconv.Itoa(lines), "--no-pager", "--output=short-iso")
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	logs := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	s.write(w, map[string]any{"ok": true, "unit": unit, "lines": logs})
}

func (s *Server) destinationsTop(w http.ResponseWriter, r *http.Request) {
	if s.destinations == nil {
		s.write(w, map[string]any{"ok": true, "items": []any{}, "total": 0, "window": "0h"})
		return
	}
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	res := s.destinations.Top(limit)
	s.write(w, map[string]any{"ok": true, "items": res.Items, "total": res.Total, "window": res.Window, "updated_at": res.UpdatedAt})
}

// userBandwidth returns the latest per-user uplink/downlink bps observed by
// the metrics collector. Powers the live bandwidth chart in the user drawer.
func (s *Server) userBandwidth(w http.ResponseWriter, r *http.Request) {
	if s.metrics == nil {
		s.write(w, map[string]any{"ok": true, "users": map[string]any{}})
		return
	}
	rates := s.metrics.LatestUserBps()
	out := make(map[string]map[string]float64, len(rates))
	for email, bps := range rates {
		out[email] = map[string]float64{"uplink_bps": bps[0], "downlink_bps": bps[1]}
	}
	s.write(w, map[string]any{"ok": true, "users": out, "updated_at": time.Now().Unix()})
}

func (s *Server) snapshotsRollback(w http.ResponseWriter, r *http.Request) {
	if !s.allowApply {
		s.fail(w, http.StatusForbidden, fmt.Errorf("rollback is disabled; start with -allow-apply"))
		return
	}
	if s.snapshots == nil {
		s.fail(w, http.StatusServiceUnavailable, fmt.Errorf("snapshots not available"))
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("id query param required"))
		return
	}
	if err := s.snapshots.Rollback(id, s.configPath, s.cfg.Server.XrayConfigPath); err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	// Reload stack config and re-apply Xray so the live state matches.
	// (Caller is expected to restart xray-stackd if structural changes happened.)
	s.recordAudit(s.actor(r), "snapshot.rollback", id, nil)
	s.write(w, map[string]any{"ok": true, "id": id, "note": "stack.json + xray config restored; restart xray-stackd to reload state"})
}
