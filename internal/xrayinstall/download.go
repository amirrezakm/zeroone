// SPDX-License-Identifier: AGPL-3.0-or-later
package xrayinstall

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LatestRelease is the trimmed shape of GitHub's "latest release"
// payload that we actually need. Tag is the canonical version string
// (e.g. "v25.2.0"); other fields are surfaced for the panel.
type LatestRelease struct {
	Tag         string    `json:"tag_name"`
	Name        string    `json:"name,omitempty"`
	PublishedAt time.Time `json:"published_at,omitempty"`
	HTMLURL     string    `json:"html_url,omitempty"`
	Assets      []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets,omitempty"`
}

type latestCacheEntry struct {
	at  time.Time
	val LatestRelease
	err error
	mu  sync.Mutex
}

// CheckLatest polls the GitHub API (or the configured mirror) and
// returns the most recent xray-core release. Results are cached for 30
// seconds to avoid hammering the API from panel refresh loops; pass
// force=true to bypass the cache.
func (i *Installer) CheckLatest(ctx context.Context, force bool) (LatestRelease, error) {
	i.latestCache.mu.Lock()
	cached := i.latestCache.val
	cachedErr := i.latestCache.err
	age := time.Since(i.latestCache.at)
	i.latestCache.mu.Unlock()
	if !force && age < time.Duration(defaultLatestCacheS)*time.Second && cached.Tag != "" {
		return cached, cachedErr
	}
	src := i.EffectiveSources()
	apiURL := src.ReleaseAPI
	if apiURL == "" {
		apiURL = DefaultReleaseAPI
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return LatestRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", src.HTTPUserAgent)
	resp, err := i.HTTPClient.Do(req)
	if err != nil {
		return LatestRelease{}, fmt.Errorf("xrayinstall: github latest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
		return LatestRelease{}, fmt.Errorf("xrayinstall: github latest: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var rel LatestRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return LatestRelease{}, fmt.Errorf("xrayinstall: parse latest: %w", err)
	}
	i.latestCache.mu.Lock()
	i.latestCache.val = rel
	i.latestCache.err = nil
	i.latestCache.at = time.Now()
	i.latestCache.mu.Unlock()
	go i.touchLastCheck(rel.Tag)
	return rel, nil
}

// AssetName picks the right Xray-core archive name for the running
// host. Matches the same heuristic the Dockerfile uses (amd64 → -64,
// arm64 → -arm64-v8a).
func AssetName() string {
	switch runtime.GOARCH {
	case "amd64":
		return "Xray-linux-64.zip"
	case "arm64":
		return "Xray-linux-arm64-v8a.zip"
	case "386":
		return "Xray-linux-32.zip"
	default:
		return fmt.Sprintf("Xray-linux-%s.zip", runtime.GOARCH)
	}
}

// releaseAssetURL constructs the upstream URL for a given version +
// asset, applying the release mirror when set. The release mirror is
// expected to mirror the same `<version>/<asset>` layout that GitHub
// uses (most Iranian PaaS mirrors and ghproxy/cdn-front do this).
func (i *Installer) releaseAssetURL(version, asset string) string {
	src := i.EffectiveSources()
	base := src.ReleaseBase
	if src.ReleaseMirror != "" {
		base = src.ReleaseMirror
	}
	base = strings.TrimRight(base, "/")
	return fmt.Sprintf("%s/%s/%s", base, version, asset)
}

// downloadWithProgress streams body→dst while ticking j.BytesDone.
// Caller must supply a deadline via ctx — this function does not set
// one itself, so a short timeout on a large download is the caller's
// responsibility.
func (i *Installer) downloadWithProgress(ctx context.Context, url, dst string, j *Job) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	src := i.EffectiveSources()
	req.Header.Set("User-Agent", src.HTTPUserAgent)
	resp, err := i.HTTPClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("xrayinstall: download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
		return 0, fmt.Errorf("xrayinstall: download %s: HTTP %d %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if j != nil && resp.ContentLength > 0 {
		j.BytesTotal = resp.ContentLength
	}
	tmp := dst + ".part"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, err
	}
	pr := &progressReader{r: resp.Body, j: j}
	n, copyErr := io.Copy(f, pr)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return n, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return n, closeErr
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return n, err
	}
	return n, nil
}

// fetchSmall pulls a small payload (≤ 64 KiB) into memory — used for
// the .dgst file which is a few hundred bytes.
func (i *Installer) fetchSmall(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	src := i.EffectiveSources()
	req.Header.Set("User-Agent", src.HTTPUserAgent)
	resp, err := i.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xrayinstall: fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
		return nil, fmt.Errorf("xrayinstall: fetch %s: HTTP %d %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<10))
}

type progressReader struct {
	r io.Reader
	j *Job
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 && p.j != nil {
		p.j.addBytes(int64(n))
	}
	return n, err
}
