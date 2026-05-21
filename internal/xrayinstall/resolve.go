// SPDX-License-Identifier: AGPL-3.0-or-later
package xrayinstall

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Resolved is the live answer to "where does xray actually live right
// now?". Either pointed at the override tree (populated by a panel
// update) or the image-baked default.
type Resolved struct {
	Binary    string `json:"binary"`     // executable path passed to exec.Command
	AssetsDir string `json:"assets_dir"` // value for XRAY_LOCATION_ASSET
	Source    string `json:"source"`     // "image" | "override"
}

// Resolve picks the effective binary + assets directory. Override wins
// when the symlink at Root/bin/xray is executable; otherwise the
// image-baked defaults take over.
func (i *Installer) Resolve() Resolved {
	if i == nil {
		return Resolved{Binary: DefaultImageBinary, AssetsDir: DefaultImageAssets, Source: "image"}
	}
	override := filepath.Join(i.Root, "bin", "xray")
	if isExecutable(override) {
		assets := filepath.Join(i.Root, "assets")
		// Geo files only count when geoip.dat actually exists. A binary
		// update that doesn't include the geo data should still fall back
		// to the image assets — otherwise xray fails to resolve geosite
		// rules.
		if !fileExists(filepath.Join(assets, "geoip.dat")) {
			assets = i.ImageAssets
			if assets == "" {
				assets = DefaultImageAssets
			}
		}
		return Resolved{Binary: override, AssetsDir: assets, Source: "override"}
	}
	binary := i.ImageBinary
	if binary == "" {
		binary = DefaultImageBinary
	}
	assets := i.ImageAssets
	if assets == "" {
		assets = DefaultImageAssets
	}
	return Resolved{Binary: binary, AssetsDir: assets, Source: "image"}
}

// ActiveBinary is a thin wrapper for callers that only need the path.
// Used by the xrayproc supervisor as its BinaryProvider getter.
func (i *Installer) ActiveBinary() string { return i.Resolve().Binary }

// ActiveAssetsDir mirrors ActiveBinary for XRAY_LOCATION_ASSET.
func (i *Installer) ActiveAssetsDir() string { return i.Resolve().AssetsDir }

// DetectVersion runs `xray version` against the given binary and
// extracts the first SemVer-looking token. Bounded to a 3s exec so it
// can be called on the hot path (panel status query) without blocking.
func DetectVersion(ctx context.Context, binary string) string {
	if binary == "" {
		return ""
	}
	if _, err := os.Stat(binary); err != nil {
		return ""
	}
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(c, binary, "version").Output()
	if err != nil {
		// Some builds need `-version`; try once.
		out, err = exec.CommandContext(c, binary, "-version").Output()
		if err != nil {
			return ""
		}
	}
	return parseVersionToken(string(out))
}

// imageVersion returns the xray version baked into the image. Detected
// lazily and cached for the life of the installer. Prefers the image
// label that the Dockerfile bakes in (ImageVer) and falls back to
// exec'ing the image binary.
func (i *Installer) imageVersion() string {
	i.imageVerOnce.Do(func() {
		if i.ImageVer != "" {
			return
		}
		i.ImageVer = DetectVersion(context.Background(), i.ImageBinary)
	})
	return i.ImageVer
}

func parseVersionToken(s string) string {
	for _, line := range strings.Split(s, "\n") {
		for _, tok := range strings.Fields(line) {
			if len(tok) < 2 {
				continue
			}
			if tok[0] == 'v' || (tok[0] >= '0' && tok[0] <= '9') {
				if looksLikeSemver(tok) {
					if tok[0] != 'v' {
						return "v" + tok
					}
					return tok
				}
			}
		}
	}
	return ""
}

func looksLikeSemver(s string) bool {
	rest := s
	if rest[0] == 'v' {
		rest = rest[1:]
	}
	parts := strings.SplitN(rest, ".", 4)
	if len(parts) < 2 {
		return false
	}
	for idx, p := range parts {
		if idx >= 3 {
			break
		}
		if p == "" {
			return false
		}
		end := len(p)
		for k, c := range p {
			if c < '0' || c > '9' {
				end = k
				break
			}
		}
		if end == 0 {
			return false
		}
	}
	return true
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func isExecutable(path string) bool {
	st, err := os.Stat(path)
	if err != nil {
		// Stat follows symlinks, so a dangling override link returns an
		// error — exactly what we want, "not usable".
		return false
	}
	if st.IsDir() {
		return false
	}
	return st.Mode().Perm()&0o111 != 0
}
