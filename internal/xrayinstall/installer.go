// SPDX-License-Identifier: AGPL-3.0-or-later

// Package xrayinstall lets the panel manage the xray binary (and the
// bundled geo data) at runtime, without rebuilding the container image.
//
// The image still ships xray at /usr/local/bin/xray + geo files in
// /usr/local/share/xray. That's the install — there is no first-boot
// download. On top of that, this package maintains a writable "override"
// tree (default: /var/lib/zeroone/xray) where panel-triggered updates
// land. Whenever the override has a usable binary, it wins; otherwise
// the daemon falls back to the image-baked copy.
//
// Layout under the override root:
//
//	bin/xray            symlink → ../versions/<ver>/xray
//	versions/<ver>/xray per-version binary (last 2 kept for rollback)
//	assets/geoip.dat    geo data (absent → fall back to image)
//	assets/geosite.dat
//	state.json          {installed_version, installed_at, source, ...}
//	tmp/                staging for downloads + uploads
package xrayinstall

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/amirrezakm/zeroone/internal/stack"
)

// Default paths and source URLs. Overridable via stack.json / env.
const (
	DefaultInstallDir   = "/var/lib/zeroone/xray"
	DefaultImageBinary  = "/usr/local/bin/xray"
	DefaultImageAssets  = "/usr/local/share/xray"
	DefaultReleaseBase  = "https://github.com/XTLS/Xray-core/releases/download"
	DefaultReleaseAPI   = "https://api.github.com/repos/XTLS/Xray-core/releases/latest"
	maxKeptVersions     = 2
	uploadMaxBytes      = 200 << 20 // 200 MB hard cap for /update/upload
	defaultLatestCacheS = 30
)

// Sources collects every URL the installer talks to. Mirror fields take
// precedence over the GitHub-flavoured base URLs.
type Sources struct {
	ReleaseBase    string        `json:"release_base"`
	ReleaseAPI     string        `json:"release_api"`
	ReleaseMirror  string        `json:"release_mirror,omitempty"`
	AssetsMirror   string        `json:"assets_mirror,omitempty"`
	HTTPUserAgent  string        `json:"-"`
	RequestTimeout time.Duration `json:"-"`
}

// Installer is the runtime owner of the override tree. Construct once
// at startup and keep around for the life of the daemon.
type Installer struct {
	Root        string // override root, e.g. /var/lib/zeroone/xray
	ImageBinary string // baked-in fallback, e.g. /usr/local/bin/xray
	ImageAssets string // baked-in geo dir, e.g. /usr/local/share/xray
	ImageVer    string // version label baked into the image (best effort)
	HTTPClient  *http.Client
	Logger      *slog.Logger
	// Restart triggers an xray restart after a successful swap. May be
	// nil (e.g. on a host install where the daemon doesn't manage xray);
	// in that case the new binary is staged on disk and picked up on
	// the next manual restart.
	Restart func(ctx context.Context) error
	// LoadConfig fetches the latest stack.Config so mirror overrides
	// from the panel are respected without a daemon restart.
	LoadConfig func() stack.Config
	// EnvSources is the env-derived default sources (release_mirror,
	// etc.). Panel overrides via stack.XrayUpdate take precedence.
	EnvSources Sources

	mu           sync.Mutex
	job          *Job
	jobHistory   []Job
	latestCache  latestCacheEntry
	imageVerOnce sync.Once
}

// New constructs an Installer. install and image paths fall back to the
// package defaults when empty.
func New(root, imageBinary, imageAssets, imageVer string, logger *slog.Logger) *Installer {
	if root == "" {
		root = DefaultInstallDir
	}
	if imageBinary == "" {
		imageBinary = DefaultImageBinary
	}
	if imageAssets == "" {
		imageAssets = DefaultImageAssets
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Installer{
		Root:        root,
		ImageBinary: imageBinary,
		ImageAssets: imageAssets,
		ImageVer:    imageVer,
		HTTPClient:  &http.Client{Timeout: 5 * time.Minute},
		Logger:      logger,
	}
}

// EnsureDirs creates the override directory skeleton with safe perms.
// Idempotent. Returns an error only if creation fails — missing
// directories are not an error on a fresh container, they're expected.
func (i *Installer) EnsureDirs() error {
	for _, sub := range []string{"bin", "versions", "assets", "tmp"} {
		if err := os.MkdirAll(filepath.Join(i.Root, sub), 0o755); err != nil {
			return fmt.Errorf("xrayinstall: mkdir %s: %w", sub, err)
		}
	}
	return nil
}

// EffectiveSources merges env defaults with the stack.XrayUpdate panel
// overrides. Panel values win; missing fields fall back to env defaults;
// finally to the hardcoded GitHub URLs.
func (i *Installer) EffectiveSources() Sources {
	src := i.EnvSources
	if src.ReleaseBase == "" {
		src.ReleaseBase = DefaultReleaseBase
	}
	if src.ReleaseAPI == "" {
		src.ReleaseAPI = DefaultReleaseAPI
	}
	if src.HTTPUserAgent == "" {
		src.HTTPUserAgent = "zeroone-xrayinstall/1"
	}
	if src.RequestTimeout == 0 {
		src.RequestTimeout = 5 * time.Minute
	}
	if i.LoadConfig != nil {
		cfg := i.LoadConfig()
		if v := cfg.XrayUpdate.ReleaseMirror; v != "" {
			src.ReleaseMirror = v
		}
		if v := cfg.XrayUpdate.AssetsMirror; v != "" {
			src.AssetsMirror = v
		}
	}
	return src
}
