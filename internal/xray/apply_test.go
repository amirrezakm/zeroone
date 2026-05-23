package xray

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/amirrezakm/zeroone/internal/stack"
	"github.com/amirrezakm/zeroone/internal/system"
)

type fakeRunner struct{}

func (fakeRunner) Run(context.Context, string, ...string) (system.Result, error) {
	return system.Result{}, nil
}

func TestPlanReportsUnchangedLiveConfig(t *testing.T) {
	cfg := minimalConfig(t)
	m := Manager{Runner: fakeRunner{}}
	rendered, err := m.Render(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Server.XrayConfigPath, append(rendered, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, _, err := m.Plan(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Changed {
		t.Fatalf("expected unchanged plan: %+v", plan)
	}
}

func TestPlanReportsChangedLiveConfig(t *testing.T) {
	cfg := minimalConfig(t)
	if err := os.WriteFile(cfg.Server.XrayConfigPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, _, err := (Manager{Runner: fakeRunner{}}).Plan(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Changed {
		t.Fatalf("expected changed plan: %+v", plan)
	}
}

func TestEnsureConfigFileWritesWhenMissing(t *testing.T) {
	cfg := minimalConfig(t)
	if _, err := os.Stat(cfg.Server.XrayConfigPath); !os.IsNotExist(err) {
		t.Fatalf("expected no file yet, got err=%v", err)
	}
	if err := EnsureConfigFile(cfg); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(cfg.Server.XrayConfigPath)
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	var doc any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("written config is not valid JSON: %v", err)
	}
}

func TestEnsureConfigFileLeavesExistingUntouched(t *testing.T) {
	cfg := minimalConfig(t)
	const sentinel = "{\"keep\":true}\n"
	if err := os.WriteFile(cfg.Server.XrayConfigPath, []byte(sentinel), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureConfigFile(cfg); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(cfg.Server.XrayConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != sentinel {
		t.Fatalf("existing file was overwritten: %q", string(b))
	}
}

func TestApplyRawWritesProvidedConfig(t *testing.T) {
	cfg := minimalConfig(t)
	m := Manager{Runner: fakeRunner{}}
	if err := os.WriteFile(cfg.Server.XrayConfigPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := []byte("{\n  \"log\": {\n    \"loglevel\": \"warning\"\n  }\n}")
	plan, err := m.ApplyRaw(context.Background(), cfg, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Changed {
		t.Fatalf("expected changed plan: %+v", plan)
	}
	got, err := os.ReadFile(cfg.Server.XrayConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(raw)+"\n" {
		t.Fatalf("live config mismatch:\n got=%q\nwant=%q", string(got), string(raw)+"\n")
	}
}

func minimalConfig(t *testing.T) stack.Config {
	t.Helper()
	dir := t.TempDir()
	return stack.Config{
		Server: stack.ServerConfig{
			XrayConfigPath: filepath.Join(dir, "config.json"),
			XrayBinary:     "xray",
			BackupDir:      filepath.Join(dir, "backups"),
		},
		Xray: stack.XrayConfig{
			LogLevel:   "warning",
			DNSServers: []string{"1.1.1.1"},
			APIPort:    10085,
			Inbounds: stack.InboundConfig{
				VLESSWSPort:    443,
				VLESSXHTTPPort: 3002,
				LocalSOCKSPort: 10808,
			},
			Users: []stack.User{{Email: "test", UUID: "00000000-0000-4000-8000-000000000000", Enabled: true}},
			Outbounds: stack.OutboundSet{
				Proxy: stack.Outbound{
					Tag:        "proxy",
					Type:       "vless-ws-tls",
					Address:    "203.0.113.1",
					Port:       443,
					UUID:       "00000000-0000-4000-8000-000000000000",
					ServerName: "example.com",
					Host:       "example.com",
					Path:       "/edge",
				},
				Fallback: stack.Outbound{
					Tag:     "priority-proxy",
					Type:    "vless-ws",
					Address: "203.0.113.2",
					Port:    80,
					UUID:    "00000000-0000-4000-8000-000000000000",
					Host:    "fallback.example.com",
					Path:    "/",
				},
			},
		},
	}
}
