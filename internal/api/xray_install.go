// SPDX-License-Identifier: AGPL-3.0-or-later
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/amirrezakm/zeroone/internal/stack"
	"github.com/amirrezakm/zeroone/internal/xrayinstall"
)

// xrayVersion returns the panel-facing status snapshot (current binary,
// version, source, latest cached, in-progress job, etc.). Read-only;
// no auth gating beyond the standard admin session.
func (s *Server) xrayVersion(w http.ResponseWriter, r *http.Request) {
	if s.xrayInstaller == nil {
		s.fail(w, http.StatusNotImplemented, errors.New("xray installer not configured"))
		return
	}
	resp := s.xrayInstaller.Status()
	s.write(w, map[string]any{"ok": true, "status": resp, "allow_apply": s.allowApply})
}

// xrayVersionCheck forces a poll of the upstream release feed. The
// installer caches the result for 30s on its own; ?force=1 bypasses
// the cache so the panel "Check for updates" button always hits the
// network.
func (s *Server) xrayVersionCheck(w http.ResponseWriter, r *http.Request) {
	if s.xrayInstaller == nil {
		s.fail(w, http.StatusNotImplemented, errors.New("xray installer not configured"))
		return
	}
	force := r.URL.Query().Get("force") != ""
	rel, err := s.xrayInstaller.CheckLatest(r.Context(), force)
	if err != nil {
		s.fail(w, http.StatusBadGateway, err)
		return
	}
	s.write(w, map[string]any{"ok": true, "latest": rel, "asset": xrayinstall.AssetName()})
}

// xrayUpdate kicks off an online update job. Body: {"version": "v25.2.0"} —
// version is optional (defaults to latest from CheckLatest).
func (s *Server) xrayUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.allowApply {
		s.fail(w, http.StatusForbidden, errors.New("xray update is disabled; start with -allow-apply"))
		return
	}
	if s.xrayInstaller == nil {
		s.fail(w, http.StatusNotImplemented, errors.New("xray installer not configured"))
		return
	}
	var req struct {
		Version string `json:"version"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.fail(w, http.StatusBadRequest, err)
			return
		}
	}
	// Honour a pinned version from stack.json: if set, an empty body
	// targets exactly that version instead of "latest". The panel
	// surfaces the pin so this is a defensive default, not a hidden
	// discovery path.
	if req.Version == "" {
		if pin := strings.TrimSpace(s.cfg.XrayUpdate.PinnedVersion); pin != "" {
			req.Version = pin
		}
	}
	job, err := s.xrayInstaller.UpdateOnline(r.Context(), req.Version)
	if err != nil {
		if errors.Is(err, xrayinstall.ErrJobInProgress) {
			s.fail(w, http.StatusConflict, err)
			return
		}
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.recordAudit(s.actor(r), "xray.update.start", req.Version, map[string]any{"job_id": job.ID, "source": "online"})
	s.write(w, map[string]any{"ok": true, "job": job})
}

// xrayUpdateUpload installs from a panel-supplied zip. multipart form:
//
//	file:    Xray-linux-<arch>.zip (required)
//	sha256:  hex digest of file (optional but recommended)
//	version: optional label, otherwise detected via `xray version`
//
// The file is streamed to /var/lib/zeroone/xray/tmp/<jobid>/ — no
// full in-memory buffering.
func (s *Server) xrayUpdateUpload(w http.ResponseWriter, r *http.Request) {
	if !s.allowApply {
		s.fail(w, http.StatusForbidden, errors.New("xray update is disabled; start with -allow-apply"))
		return
	}
	if s.xrayInstaller == nil {
		s.fail(w, http.StatusNotImplemented, errors.New("xray installer not configured"))
		return
	}
	if r.ContentLength > xrayUploadMaxBytes {
		s.fail(w, http.StatusRequestEntityTooLarge, fmt.Errorf("upload too large (%d bytes); cap is %d", r.ContentLength, xrayUploadMaxBytes))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, xrayUploadMaxBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("parse multipart: %w", err))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("file field required: %w", err))
		return
	}
	defer file.Close()
	sha := strings.ToLower(strings.TrimSpace(r.FormValue("sha256")))
	version := strings.TrimSpace(r.FormValue("version"))

	if err := s.xrayInstaller.EnsureDirs(); err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	uploadName := filepath.Base(header.Filename)
	if uploadName == "" {
		uploadName = "upload.zip"
	}
	stageDir := filepath.Join(s.xrayInstaller.Root, "tmp", "upload-"+uploadName)
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	dst := filepath.Join(stageDir, uploadName)
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := io.Copy(out, file); err != nil {
		_ = out.Close()
		_ = os.RemoveAll(stageDir)
		s.fail(w, http.StatusInternalServerError, fmt.Errorf("save upload: %w", err))
		return
	}
	if err := out.Close(); err != nil {
		_ = os.RemoveAll(stageDir)
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	job, err := s.xrayInstaller.UpdateFromUpload(r.Context(), dst, sha, version)
	if err != nil {
		_ = os.RemoveAll(stageDir)
		if errors.Is(err, xrayinstall.ErrJobInProgress) {
			s.fail(w, http.StatusConflict, err)
			return
		}
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.recordAudit(s.actor(r), "xray.update.upload", uploadName, map[string]any{"job_id": job.ID, "sha256_provided": sha != ""})
	s.write(w, map[string]any{"ok": true, "job": job})
}

// xrayUpdateStatus returns the current or last job snapshot — used by
// the panel to poll a running update's progress.
func (s *Server) xrayUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if s.xrayInstaller == nil {
		s.fail(w, http.StatusNotImplemented, errors.New("xray installer not configured"))
		return
	}
	st := s.xrayInstaller.Status()
	s.write(w, map[string]any{"ok": true, "job": st.Job, "last_job": st.LastJob})
}

func (s *Server) xrayRollback(w http.ResponseWriter, r *http.Request) {
	if !s.allowApply {
		s.fail(w, http.StatusForbidden, errors.New("rollback is disabled; start with -allow-apply"))
		return
	}
	if s.xrayInstaller == nil {
		s.fail(w, http.StatusNotImplemented, errors.New("xray installer not configured"))
		return
	}
	if err := s.xrayInstaller.Rollback(r.Context()); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	s.recordAudit(s.actor(r), "xray.rollback", "", nil)
	s.write(w, map[string]any{"ok": true, "status": s.xrayInstaller.Status()})
}

func (s *Server) xrayResetToImage(w http.ResponseWriter, r *http.Request) {
	if !s.allowApply {
		s.fail(w, http.StatusForbidden, errors.New("reset is disabled; start with -allow-apply"))
		return
	}
	if s.xrayInstaller == nil {
		s.fail(w, http.StatusNotImplemented, errors.New("xray installer not configured"))
		return
	}
	if err := s.xrayInstaller.ResetToImage(r.Context()); err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.recordAudit(s.actor(r), "xray.reset_to_image", "", nil)
	s.write(w, map[string]any{"ok": true, "status": s.xrayInstaller.Status()})
}

func (s *Server) xrayUpdateConfigGet(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.currentConfig(w)
	if !ok {
		return
	}
	view := xrayUpdateConfigView(cfg.XrayUpdate)
	if s.xrayInstaller != nil {
		view["effective_sources"] = s.xrayInstaller.EffectiveSources()
	}
	s.write(w, map[string]any{"ok": true, "config": view})
}

func (s *Server) xrayUpdateConfigPut(w http.ResponseWriter, r *http.Request) {
	if !s.allowApply {
		s.fail(w, http.StatusForbidden, errors.New("config write is disabled; start with -allow-apply"))
		return
	}
	cfg, ok := s.currentConfig(w)
	if !ok {
		return
	}
	var req struct {
		ReleaseMirror *string `json:"release_mirror"`
		AssetsMirror  *string `json:"assets_mirror"`
		PinnedVersion *string `json:"pinned_version"`
		AutoCheck     *bool   `json:"auto_check"`
		IncludeGeo    *bool   `json:"include_geo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	patch := cfg.XrayUpdate
	if req.ReleaseMirror != nil {
		patch.ReleaseMirror = *req.ReleaseMirror
	}
	if req.AssetsMirror != nil {
		patch.AssetsMirror = *req.AssetsMirror
	}
	if req.PinnedVersion != nil {
		patch.PinnedVersion = *req.PinnedVersion
	}
	if req.AutoCheck != nil {
		patch.AutoCheck = req.AutoCheck
	}
	if req.IncludeGeo != nil {
		patch.IncludeGeo = req.IncludeGeo
	}
	cfg.SetXrayUpdateConfig(patch)
	if err := stack.Save(s.configPath, cfg); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	s.cfg = cfg
	s.recordAudit(s.actor(r), "xray.update.config", "", map[string]any{
		"release_mirror": cfg.XrayUpdate.ReleaseMirror,
		"assets_mirror":  cfg.XrayUpdate.AssetsMirror,
		"pinned":         cfg.XrayUpdate.PinnedVersion,
	})
	view := xrayUpdateConfigView(cfg.XrayUpdate)
	if s.xrayInstaller != nil {
		view["effective_sources"] = s.xrayInstaller.EffectiveSources()
	}
	s.write(w, map[string]any{"ok": true, "config": view})
}

func xrayUpdateConfigView(c stack.XrayUpdateConfig) map[string]any {
	auto := true
	geo := true
	if c.AutoCheck != nil {
		auto = *c.AutoCheck
	}
	if c.IncludeGeo != nil {
		geo = *c.IncludeGeo
	}
	return map[string]any{
		"release_mirror": c.ReleaseMirror,
		"assets_mirror":  c.AssetsMirror,
		"pinned_version": c.PinnedVersion,
		"auto_check":     auto,
		"include_geo":    geo,
	}
}

const xrayUploadMaxBytes int64 = 200 << 20
