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
		if cfg.Server.AdminListen == "" || cfg.Server.AdminListen == "127.0.0.1:8091" {
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
		if cfg.Server.FailoverStatePath == "" || cfg.Server.FailoverStatePath == "/var/lib/xray-stack/failover-state.json" {
			cfg.Server.FailoverStatePath = filepath.Join(v, "failover-state.json")
		}
	}
}
