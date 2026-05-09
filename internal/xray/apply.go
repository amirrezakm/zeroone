package xray

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
	"github.com/sakhtar/xray-stack-zeroone/internal/system"
)

type ApplyPlan struct {
	ConfigPath string `json:"config_path"`
	BackupPath string `json:"backup_path"`
	Changed    bool   `json:"changed"`
}

type Manager struct{ Runner system.Runner }

func (m Manager) Render(cfg stack.Config) ([]byte, error) {
	return json.MarshalIndent(Generate(cfg), "", "  ")
}

func (m Manager) Validate(ctx context.Context, cfg stack.Config, rendered []byte) error {
	tmp, err := os.CreateTemp("", "xray-stackd-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(rendered); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	runner := m.Runner
	if runner == nil {
		runner = system.ExecRunner{Timeout: 15 * time.Second}
	}
	res, err := runner.Run(ctx, cfg.Server.XrayBinary, "run", "-test", "-config", tmp.Name())
	if err != nil {
		return fmt.Errorf("xray config test failed: %w: %s%s", err, res.Stdout, res.Stderr)
	}
	return nil
}

func (m Manager) Plan(ctx context.Context, cfg stack.Config) (ApplyPlan, []byte, error) {
	rendered, err := m.Render(cfg)
	if err != nil {
		return ApplyPlan{}, nil, err
	}
	if err := m.Validate(ctx, cfg, rendered); err != nil {
		return ApplyPlan{}, nil, err
	}
	plan := ApplyPlan{ConfigPath: cfg.Server.XrayConfigPath}
	current, _ := os.ReadFile(cfg.Server.XrayConfigPath)
	plan.Changed = string(current) != string(rendered)+"\n" && string(current) != string(rendered)
	return plan, rendered, nil
}

func (m Manager) Apply(ctx context.Context, cfg stack.Config) (ApplyPlan, error) {
	plan, rendered, err := m.Plan(ctx, cfg)
	if err != nil {
		return ApplyPlan{}, err
	}
	if !plan.Changed {
		return plan, nil
	}
	backupDir := cfg.Server.BackupDir
	if backupDir == "" {
		backupDir = "/root/xray-audit-backups"
	}
	stamp := time.Now().Format("20060102-150405-go-apply")
	backupPath := filepath.Join(backupDir, stamp, "config.json")
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
		return plan, err
	}
	current, _ := os.ReadFile(cfg.Server.XrayConfigPath)
	if len(current) > 0 {
		if err := os.WriteFile(backupPath, current, 0o600); err != nil {
			return plan, err
		}
	}
	tmp := cfg.Server.XrayConfigPath + ".tmp-go-apply"
	if err := os.WriteFile(tmp, append(rendered, '\n'), 0o644); err != nil {
		return plan, err
	}
	if err := os.Rename(tmp, cfg.Server.XrayConfigPath); err != nil {
		return plan, err
	}
	plan.BackupPath = backupPath
	runner := m.Runner
	if runner == nil {
		runner = system.ExecRunner{Timeout: 20 * time.Second}
	}
	if _, err := runner.Run(ctx, "systemctl", "restart", "xray.service"); err != nil {
		return plan, fmt.Errorf("restart xray: %w", err)
	}
	return plan, nil
}
