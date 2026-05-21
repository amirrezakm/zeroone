// SPDX-License-Identifier: AGPL-3.0-or-later
package xrayinstall

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrJobInProgress is returned when an update is requested while
// another is already running. Callers (the API layer) surface it as
// HTTP 409.
var ErrJobInProgress = errors.New("xrayinstall: another update is already running")

// Status is the panel-facing snapshot returned by GET /api/xray/version.
type Status struct {
	Active        Resolved      `json:"active"`
	ActiveVersion string        `json:"active_version"`
	ImageVersion  string        `json:"image_version"`
	State         State         `json:"state"`
	Versions      []string      `json:"versions"`
	Job           *Job          `json:"job,omitempty"`
	LastJob       *Job          `json:"last_job,omitempty"`
	Latest        LatestRelease `json:"latest,omitempty"`
	Sources       Sources       `json:"sources"`
	HasOverride   bool          `json:"has_override"`
}

// Status returns a snapshot of where the daemon stands. Cheap enough
// to call from a panel refresh loop (the version detection is cached
// after the first call).
func (i *Installer) Status() Status {
	st, _ := i.LoadState()
	resolved := i.Resolve()
	active := DetectVersion(context.Background(), resolved.Binary)
	if active == "" && resolved.Source == "override" {
		active = st.InstalledVersion
	}
	if active == "" {
		active = i.imageVersion()
	}
	out := Status{
		Active:        resolved,
		ActiveVersion: active,
		ImageVersion:  i.imageVersion(),
		State:         st,
		Versions:      i.ListVersions(),
		Sources:       i.EffectiveSources(),
		HasOverride:   resolved.Source == "override",
	}
	i.mu.Lock()
	if i.job != nil {
		snap := i.job.snapshot()
		out.Job = &snap
	} else if n := len(i.jobHistory); n > 0 {
		last := i.jobHistory[n-1]
		out.LastJob = &last
	}
	if last := i.latestCache.val; last.Tag != "" {
		out.Latest = last
	}
	i.mu.Unlock()
	return out
}

// UpdateOnline starts a background job that downloads and installs the
// requested version (or the latest, when version is empty). Returns
// the freshly-created Job snapshot so the panel can start polling.
func (i *Installer) UpdateOnline(ctx context.Context, version string) (Job, error) {
	if version != "" {
		if err := ValidateVersionToken(version); err != nil {
			return Job{}, err
		}
	}
	job, err := i.beginJob("online")
	if err != nil {
		return Job{}, err
	}
	if version != "" {
		job.TargetVersion = version
	}
	bg := context.Background()
	go i.runOnline(bg, job, version)
	return job.snapshot(), nil
}

// UpdateFromUpload starts a background job that installs from a
// caller-supplied zip already on disk. expectedSHA may be empty, in
// which case integrity is implicit (uploaded by an authenticated
// admin). When set, sha mismatch fails the job before any swap.
func (i *Installer) UpdateFromUpload(ctx context.Context, zipPath, expectedSHA, version string) (Job, error) {
	if version != "" {
		if err := ValidateVersionToken(version); err != nil {
			return Job{}, err
		}
	}
	job, err := i.beginJob("upload")
	if err != nil {
		return Job{}, err
	}
	job.TargetVersion = version
	bg := context.Background()
	go i.runUpload(bg, job, zipPath, expectedSHA, version)
	return job.snapshot(), nil
}

// beginJob acquires the single-job mutex and seeds a new Job record.
// Caller is responsible for calling endJob in every exit path.
func (i *Installer) beginJob(source string) (*Job, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.job != nil && i.job.Phase != PhaseDone && i.job.Phase != PhaseFailed {
		return nil, ErrJobInProgress
	}
	id := randHex(8)
	j := &Job{ID: id, Phase: PhaseQueued, StartedAt: time.Now().Unix(), Source: source}
	i.job = j
	return j, nil
}

func (i *Installer) endJob(j *Job) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if j == nil {
		return
	}
	final := j.snapshot()
	i.jobHistory = append(i.jobHistory, final)
	if len(i.jobHistory) > 8 {
		i.jobHistory = i.jobHistory[len(i.jobHistory)-8:]
	}
	i.job = nil
}

func (i *Installer) runOnline(ctx context.Context, j *Job, requested string) {
	defer i.endJob(j)
	dlCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	version := requested
	if version == "" {
		j.setPhase(PhaseDownloading)
		latest, err := i.CheckLatest(dlCtx, true)
		if err != nil {
			i.fail(j, fmt.Errorf("check latest: %w", err))
			return
		}
		version = latest.Tag
		j.TargetVersion = version
	}
	if version == "" {
		i.fail(j, errors.New("no target version"))
		return
	}
	if err := ValidateVersionToken(version); err != nil {
		i.fail(j, err)
		return
	}
	asset := AssetName()
	zipURL := i.releaseAssetURL(version, asset)
	dgstURL := zipURL + ".dgst"

	if err := i.EnsureDirs(); err != nil {
		i.fail(j, err)
		return
	}
	tmpDir := filepath.Join(i.Root, "tmp", j.ID)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		i.fail(j, err)
		return
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	j.setPhase(PhaseDownloading)
	zipPath := filepath.Join(tmpDir, asset)
	if _, err := i.downloadWithProgress(dlCtx, zipURL, zipPath, j); err != nil {
		i.fail(j, err)
		return
	}

	j.setPhase(PhaseVerifying)
	gotSHA, err := FileSHA256(zipPath)
	if err != nil {
		i.fail(j, err)
		return
	}
	dgstBody, err := i.fetchSmall(dlCtx, dgstURL)
	if err != nil {
		// Treat missing .dgst as a hard error online — we won't trust an
		// unverified binary in production.
		i.fail(j, fmt.Errorf("fetch %s: %w", dgstURL, err))
		return
	}
	wantSHA, err := ParseDigest(dgstBody)
	if err != nil {
		i.fail(j, err)
		return
	}
	if !strings.EqualFold(gotSHA, wantSHA) {
		i.fail(j, fmt.Errorf("sha256 mismatch: got %s want %s", gotSHA, wantSHA))
		return
	}

	i.applyExtracted(ctx, j, zipPath, tmpDir, version, "online", gotSHA)
}

func (i *Installer) runUpload(ctx context.Context, j *Job, zipPath, expectedSHA, version string) {
	defer i.endJob(j)
	tmpDir := filepath.Join(i.Root, "tmp", j.ID)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	j.setPhase(PhaseVerifying)
	gotSHA, err := FileSHA256(zipPath)
	if err != nil {
		i.fail(j, err)
		return
	}
	if expectedSHA != "" && !strings.EqualFold(gotSHA, expectedSHA) {
		i.fail(j, fmt.Errorf("sha256 mismatch: got %s expected %s", gotSHA, expectedSHA))
		return
	}
	i.applyExtracted(ctx, j, zipPath, tmpDir, version, "upload", gotSHA)
}

func (i *Installer) applyExtracted(ctx context.Context, j *Job, zipPath, stageDir, version, source, sha string) {
	j.setPhase(PhaseStaging)
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		i.fail(j, err)
		return
	}
	extracted, err := extractZip(zipPath, stageDir)
	if err != nil {
		i.fail(j, err)
		return
	}
	if err := smokeTest(ctx, extracted.BinaryPath); err != nil {
		i.fail(j, err)
		return
	}
	if version == "" {
		version = DetectVersion(ctx, extracted.BinaryPath)
		j.TargetVersion = version
	}
	j.setPhase(PhaseSwapping)
	if err := i.commitInstall(extracted, version, source, sha); err != nil {
		i.fail(j, err)
		return
	}
	j.setPhase(PhaseRestarting)
	if i.Restart != nil {
		restartCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := i.Restart(restartCtx); err != nil {
			// Don't fail the job — the new binary is on disk; the admin
			// can restart manually. Stash the warning in Error so the
			// panel surfaces it.
			j.Error = "restart failed: " + err.Error()
		}
	}
	j.finish(PhaseDone, j.Error)
}

func (i *Installer) fail(j *Job, err error) {
	if i.Logger != nil {
		i.Logger.Error("xrayinstall: job failed", "id", j.ID, "err", err)
	}
	j.finish(PhaseFailed, err.Error())
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
