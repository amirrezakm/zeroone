// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	_ "embed"
	"os"
	"path/filepath"

	"github.com/amirrezakm/zeroone/internal/stack"
)

//go:embed minimal-stack.json
var minimalStackJSON []byte

// Template/legacy defaults that env overrides are allowed to replace.
// Operator-pinned JSON values (anything not in these sets) are left
// alone. Keep the container-template values here in sync with
// minimal-stack.json.
var (
	defaultAdminListens = map[string]bool{
		"":               true,
		"127.0.0.1:8091": true, // legacy host default
		"0.0.0.0:8000":   true, // minimal-stack.json container template
	}
	defaultFailoverStatePaths = map[string]bool{
		"": true,
		"/var/lib/xray-stack/failover-state.json": true, // legacy host default
		"/var/lib/zeroone/failover-state.json":    true, // minimal-stack.json container template
	}
)

// writeMinimalConfig writes the embedded minimal stack.json template to
// path. The template is JSON-validated against the Config schema as a
// sanity check on every build.
func writeMinimalConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, minimalStackJSON, 0o600)
}

// applyEnvOverrides fills in default-flavored fields from environment
// variables when set. Operator-pinned JSON values are left alone — env
// only wins over empty/default values. Used by the container build so
// the operator can tweak paths via .env without editing the JSON file.
func applyEnvOverrides(cfg *stack.Config) {
	if v := os.Getenv("ZEROONE_ADMIN_LISTEN"); v != "" {
		if defaultAdminListens[cfg.Server.AdminListen] {
			cfg.Server.AdminListen = v
		}
	}
	if v := os.Getenv("ZEROONE_UI_PATH"); v != "" && cfg.Server.UIPath == "" {
		cfg.Server.UIPath = v
	}
	if v := os.Getenv("ZEROONE_XRAY_BINARY"); v != "" && cfg.Server.XrayBinary == "" {
		cfg.Server.XrayBinary = v
	}
	if v := os.Getenv("ZEROONE_XRAY_CONFIG_PATH"); v != "" && cfg.Server.XrayConfigPath == "" {
		cfg.Server.XrayConfigPath = v
	}
	if v := os.Getenv("ZEROONE_BANDWIDTH_DEVICE"); v != "" && cfg.Server.BandwidthDevice == "" {
		cfg.Server.BandwidthDevice = v
	}
	if v := os.Getenv("ZEROONE_STATE_DIR"); v != "" {
		if defaultFailoverStatePaths[cfg.Server.FailoverStatePath] {
			cfg.Server.FailoverStatePath = filepath.Join(v, "failover-state.json")
		}
	}
	if v := os.Getenv("ZEROONE_XRAY_INSTALL_DIR"); v != "" && cfg.Server.XrayInstallDir == "" {
		cfg.Server.XrayInstallDir = v
	}
	if v := os.Getenv("ZEROONE_XRAY_RELEASE_MIRROR"); v != "" && cfg.XrayUpdate.ReleaseMirror == "" {
		cfg.XrayUpdate.ReleaseMirror = v
	}
	if v := os.Getenv("ZEROONE_XRAY_ASSETS_MIRROR"); v != "" && cfg.XrayUpdate.AssetsMirror == "" {
		cfg.XrayUpdate.AssetsMirror = v
	}
}
