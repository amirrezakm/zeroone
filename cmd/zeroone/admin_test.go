// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amirrezakm/zeroone/internal/auth"
	"github.com/amirrezakm/zeroone/internal/stack"
)

func writeMinimalConfigForTest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "stack.json")
	if err := os.WriteFile(path, minimalStackJSON, 0o600); err != nil {
		t.Fatalf("seed minimal: %v", err)
	}
	return path
}

func TestAdminAddListAndResetPassword(t *testing.T) {
	cfg := writeMinimalConfigForTest(t)

	if code := adminAdd([]string{"-config", cfg, "-username", "alice", "-password", "s3cret123"}); code != 0 {
		t.Fatalf("admin add: exit=%d", code)
	}
	// duplicate add should fail
	if code := adminAdd([]string{"-config", cfg, "-username", "alice", "-password", "x"}); code == 0 {
		t.Fatal("expected duplicate add to fail")
	}
	// load and verify
	c, err := stack.Load(cfg)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(c.Panel.Admins) != 1 {
		t.Fatalf("expected 1 admin, got %d", len(c.Panel.Admins))
	}
	if c.Panel.Admins[0].Username != "alice" {
		t.Fatalf("expected alice, got %q", c.Panel.Admins[0].Username)
	}
	if !auth.VerifyPassword("s3cret123", c.Panel.Admins[0].PasswordHash) {
		t.Fatal("password hash does not verify against original password")
	}

	// reset
	if code := adminResetPassword([]string{"-config", cfg, "-username", "alice", "-password", "new-passw0rd"}); code != 0 {
		t.Fatalf("admin reset-password: exit=%d", code)
	}
	c, _ = stack.Load(cfg)
	if !auth.VerifyPassword("new-passw0rd", c.Panel.Admins[0].PasswordHash) {
		t.Fatal("password hash does not verify against new password")
	}
	if auth.VerifyPassword("s3cret123", c.Panel.Admins[0].PasswordHash) {
		t.Fatal("old password still verifies; reset did not take effect")
	}
}

func TestAdminListEmptyConfig(t *testing.T) {
	cfg := writeMinimalConfigForTest(t)
	if code := adminList([]string{"-config", cfg}); code != 0 {
		t.Fatalf("admin list: exit=%d", code)
	}
}

func TestWriteMinimalConfigIsLoadable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stack.json")
	if err := writeMinimalConfig(path); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := stack.Load(path); err != nil {
		t.Fatalf("load minimal: %v", err)
	}
}
