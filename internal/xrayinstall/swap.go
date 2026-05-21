// SPDX-License-Identifier: AGPL-3.0-or-later
package xrayinstall

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// extractedAssets is what we pull out of an Xray-core release zip.
// All paths are absolute under the staging dir.
type extractedAssets struct {
	BinaryPath  string
	GeoIPPath   string
	GeoSitePath string
}

// extractZip walks the archive, picks out the xray binary + bundled
// geo data, and writes them flat into stageDir. Unrelated files
// (LICENSE, README, etc.) are skipped to keep the staging tree clean.
func extractZip(zipPath, stageDir string) (extractedAssets, error) {
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		return extractedAssets{}, err
	}
	z, err := zip.OpenReader(zipPath)
	if err != nil {
		return extractedAssets{}, fmt.Errorf("xrayinstall: open zip: %w", err)
	}
	defer func() { _ = z.Close() }()
	var out extractedAssets
	for _, f := range z.File {
		name := filepath.Base(f.Name)
		switch name {
		case "xray", "xray.exe":
			dst := filepath.Join(stageDir, "xray")
			if err := copyZipEntry(f, dst, 0o755); err != nil {
				return out, err
			}
			out.BinaryPath = dst
		case "geoip.dat":
			dst := filepath.Join(stageDir, "geoip.dat")
			if err := copyZipEntry(f, dst, 0o644); err != nil {
				return out, err
			}
			out.GeoIPPath = dst
		case "geosite.dat":
			dst := filepath.Join(stageDir, "geosite.dat")
			if err := copyZipEntry(f, dst, 0o644); err != nil {
				return out, err
			}
			out.GeoSitePath = dst
		}
	}
	if out.BinaryPath == "" {
		return out, fmt.Errorf("xrayinstall: archive missing `xray` binary")
	}
	return out, nil
}

func copyZipEntry(f *zip.File, dst string, mode os.FileMode) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	w, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, rc); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return nil
}

// smokeTest validates that a freshly-staged binary is usable. We run
// `xray version` (cheap, no config) — running `-test` against the live
// config requires knowing the in-flight config path and risks false
// positives if the running config is mid-edit, so we keep this
// minimal. The Restart-time health gate handles "did the new binary
// actually serve traffic" instead.
func smokeTest(ctx context.Context, binary string) error {
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(c, binary, "version").CombinedOutput()
	if err != nil {
		out2, err2 := exec.CommandContext(c, binary, "-version").CombinedOutput()
		if err2 == nil {
			return nil
		}
		_ = out
		return fmt.Errorf("xrayinstall: smoke test failed: %w (%s)", err, strings.TrimSpace(string(out2)))
	}
	return nil
}

// commitInstall is the atomic write portion of an update: take the
// extracted assets, move them into the override tree, flip the symlink,
// rotate older versions, and persist state.json.
//
// On error, partial state is left on disk for diagnosis but never
// pointed at by the live symlink — i.e. xray keeps running on the old
// version.
func (i *Installer) commitInstall(extracted extractedAssets, version, source, binarySHA string) error {
	if err := i.EnsureDirs(); err != nil {
		return err
	}
	if version == "" {
		// Detect version from the staged binary as a fallback (e.g.
		// when an uploader didn't pass a version label).
		version = DetectVersion(context.Background(), extracted.BinaryPath)
	}
	if version == "" {
		return fmt.Errorf("xrayinstall: cannot determine target version")
	}
	// Inline regex sanitiser at the path-construction site. CodeQL's
	// go/path-injection query treats regexp.Regexp.MatchString as a
	// sanitiser only when it appears on the taint path immediately
	// before the join, so we re-check here even though
	// ValidateVersionToken already did the same regex match upstream.
	if !versionTokenRE.MatchString(version) || version == "." || version == ".." || strings.Contains(version, "..") {
		return errInvalidVersion
	}
	prev, _ := i.LoadState()
	versionsRoot := filepath.Join(i.Root, "versions")
	versionDir := filepath.Join(versionsRoot, version)
	if err := os.RemoveAll(versionDir); err != nil {
		return err
	}
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		return err
	}
	binTarget := filepath.Join(versionDir, "xray")
	if err := os.Rename(extracted.BinaryPath, binTarget); err != nil {
		return err
	}
	if err := os.Chmod(binTarget, 0o755); err != nil {
		return err
	}

	// Symlink: bin/xray.new → ../versions/<ver>/xray, then atomic rename.
	binLinkDir := filepath.Join(i.Root, "bin")
	if err := os.MkdirAll(binLinkDir, 0o755); err != nil {
		return err
	}
	linkTarget := filepath.Join("..", "versions", version, "xray")
	tmpLink := filepath.Join(binLinkDir, "xray.new")
	_ = os.Remove(tmpLink)
	if err := os.Symlink(linkTarget, tmpLink); err != nil {
		return err
	}
	if err := os.Rename(tmpLink, filepath.Join(binLinkDir, "xray")); err != nil {
		_ = os.Remove(tmpLink)
		return err
	}

	// Geo data: atomic per file. Absent in extracted means "leave the
	// existing assets dir alone, fall back to image".
	state := State{
		InstalledVersion: version,
		InstalledAt:      time.Now().Unix(),
		Source:           source,
		BinarySHA256:     binarySHA,
		PreviousVersion:  prev.InstalledVersion,
		GeoIPSHA256:      prev.GeoIPSHA256,
		GeoSiteSHA256:    prev.GeoSiteSHA256,
		LastCheckUnix:    prev.LastCheckUnix,
		LastCheckLatest:  prev.LastCheckLatest,
	}
	assetsDir := filepath.Join(i.Root, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		return err
	}
	for _, m := range []struct {
		from, name string
		shaOut     *string
	}{
		{extracted.GeoIPPath, "geoip.dat", &state.GeoIPSHA256},
		{extracted.GeoSitePath, "geosite.dat", &state.GeoSiteSHA256},
	} {
		if m.from == "" {
			continue
		}
		dst := filepath.Join(assetsDir, m.name)
		dstTmp := dst + ".new"
		if err := os.Rename(m.from, dstTmp); err != nil {
			return err
		}
		if err := os.Rename(dstTmp, dst); err != nil {
			return err
		}
		if sha, err := FileSHA256(dst); err == nil {
			*m.shaOut = sha
		}
	}

	if err := i.SaveState(state); err != nil {
		return err
	}
	i.rotateOldVersions(version)
	return nil
}

// rotateOldVersions keeps the current + previous version dirs and
// removes anything older. We never delete the version the symlink
// currently points at, even if it lands outside the keep window.
func (i *Installer) rotateOldVersions(keep string) {
	root := filepath.Join(i.Root, "versions")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	type entry struct {
		name string
		t    time.Time
	}
	items := make([]entry, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		items = append(items, entry{name: e.Name(), t: info.ModTime()})
	}
	sort.Slice(items, func(a, b int) bool { return items[a].t.After(items[b].t) })
	// items[0] is the most-recently-touched dir (the one we just
	// installed). We keep up to maxKeptVersions newest dirs.
	for idx, it := range items {
		if idx < maxKeptVersions {
			continue
		}
		if it.name == keep {
			continue
		}
		_ = os.RemoveAll(filepath.Join(root, it.name))
	}
}

// ListVersions returns the on-disk versions sorted newest-first.
func (i *Installer) ListVersions() []string {
	root := filepath.Join(i.Root, "versions")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	type entry struct {
		name string
		t    time.Time
	}
	items := make([]entry, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		items = append(items, entry{name: e.Name(), t: info.ModTime()})
	}
	sort.Slice(items, func(a, b int) bool { return items[a].t.After(items[b].t) })
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.name)
	}
	return out
}

// Rollback flips the bin/xray symlink to the previous version dir.
// Returns an error when no prior version is present; in that case the
// caller can offer ResetToImage as the next step.
func (i *Installer) Rollback(ctx context.Context) error {
	versions := i.ListVersions()
	if len(versions) < 2 {
		return errors.New("xrayinstall: no previous version to roll back to")
	}
	// versions[0] is the current. versions[1] is the previous.
	target := versions[1]
	binLinkDir := filepath.Join(i.Root, "bin")
	tmpLink := filepath.Join(binLinkDir, "xray.new")
	_ = os.Remove(tmpLink)
	if err := os.Symlink(filepath.Join("..", "versions", target, "xray"), tmpLink); err != nil {
		return err
	}
	if err := os.Rename(tmpLink, filepath.Join(binLinkDir, "xray")); err != nil {
		return err
	}
	st, _ := i.LoadState()
	st.PreviousVersion = st.InstalledVersion
	st.InstalledVersion = target
	st.InstalledAt = time.Now().Unix()
	st.Source = "rollback"
	st.BinarySHA256 = ""
	if err := i.SaveState(st); err != nil {
		return err
	}
	if i.Restart != nil {
		return i.Restart(ctx)
	}
	return nil
}

// ResetToImage wipes the override tree so the daemon goes back to the
// image-baked binary. Used when an admin wants a clean slate after a
// bad update.
func (i *Installer) ResetToImage(ctx context.Context) error {
	for _, sub := range []string{"bin", "versions", "assets"} {
		if err := os.RemoveAll(filepath.Join(i.Root, sub)); err != nil {
			return err
		}
	}
	_ = os.Remove(filepath.Join(i.Root, "state.json"))
	if err := i.EnsureDirs(); err != nil {
		return err
	}
	if i.Restart != nil {
		return i.Restart(ctx)
	}
	return nil
}
