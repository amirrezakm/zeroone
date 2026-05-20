// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/amirrezakm/zeroone/internal/stack"
)

func TestApplyEnvOverrides_ContainerTemplateDefaults(t *testing.T) {
	var cfg stack.Config
	if err := json.Unmarshal(minimalStackJSON, &cfg); err != nil {
		t.Fatalf("unmarshal minimal-stack: %v", err)
	}
	if cfg.Server.AdminListen != "0.0.0.0:8000" {
		t.Fatalf("template admin_listen drifted: %q", cfg.Server.AdminListen)
	}
	if cfg.Server.FailoverStatePath != "/var/lib/zeroone/failover-state.json" {
		t.Fatalf("template failover_state_path drifted: %q", cfg.Server.FailoverStatePath)
	}

	t.Setenv("ZEROONE_ADMIN_LISTEN", "127.0.0.1:9000")
	t.Setenv("ZEROONE_STATE_DIR", "/srv/zeroone")

	applyEnvOverrides(&cfg)

	if cfg.Server.AdminListen != "127.0.0.1:9000" {
		t.Errorf("ZEROONE_ADMIN_LISTEN ignored: got %q", cfg.Server.AdminListen)
	}
	want := filepath.Join("/srv/zeroone", "failover-state.json")
	if cfg.Server.FailoverStatePath != want {
		t.Errorf("ZEROONE_STATE_DIR ignored: got %q want %q", cfg.Server.FailoverStatePath, want)
	}
}

func TestApplyEnvOverrides_PreservesOperatorValues(t *testing.T) {
	cfg := stack.Config{}
	cfg.Server.AdminListen = "192.0.2.10:8443"                         // operator-pinned
	cfg.Server.FailoverStatePath = "/etc/zeroone-custom/failover.json" // operator-pinned

	t.Setenv("ZEROONE_ADMIN_LISTEN", "127.0.0.1:9000")
	t.Setenv("ZEROONE_STATE_DIR", "/srv/zeroone")

	applyEnvOverrides(&cfg)

	if cfg.Server.AdminListen != "192.0.2.10:8443" {
		t.Errorf("operator AdminListen overridden: %q", cfg.Server.AdminListen)
	}
	if cfg.Server.FailoverStatePath != "/etc/zeroone-custom/failover.json" {
		t.Errorf("operator FailoverStatePath overridden: %q", cfg.Server.FailoverStatePath)
	}
}
