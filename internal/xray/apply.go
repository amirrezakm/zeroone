package xray

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/amirrezakm/zeroone/internal/stack"
	"github.com/amirrezakm/zeroone/internal/system"
)

type ApplyPlan struct {
	ConfigPath string `json:"config_path"`
	BackupPath string `json:"backup_path"`
	Changed    bool   `json:"changed"`
}

type Manager struct{ Runner system.Runner }

// Restarter restarts the running xray process after a new config has
// been written. The default implementation shells out to
// `systemctl restart xray.service`; container builds swap in a
// child-process supervisor via SetRestarter.
type Restarter interface {
	Restart(ctx context.Context, runner system.Runner) error
}

var activeRestarter Restarter = systemctlRestarter{}

// SetRestarter overrides the default systemctl-backed restarter. Called
// once during daemon startup when -manage-xray is set.
func SetRestarter(r Restarter) {
	if r != nil {
		activeRestarter = r
	}
}

// BinaryResolver returns the runtime xray binary path. The default
// honours cfg.Server.XrayBinary directly; xrayinstall.Installer
// overrides this so panel-driven updates take effect on the next
// Validate / Restart without mutating the persisted config.
type BinaryResolver func(cfg stack.Config) string

var activeBinaryResolver BinaryResolver

// SetBinaryResolver wires a custom resolver. Called once during daemon
// startup after the installer is constructed.
func SetBinaryResolver(r BinaryResolver) { activeBinaryResolver = r }

func resolveBinary(cfg stack.Config) string {
	if activeBinaryResolver != nil {
		if p := activeBinaryResolver(cfg); p != "" {
			return p
		}
	}
	return cfg.Server.XrayBinary
}

type systemctlRestarter struct{}

func (systemctlRestarter) Restart(ctx context.Context, runner system.Runner) error {
	if runner == nil {
		runner = system.ExecRunner{Timeout: 20 * time.Second}
	}
	_, err := runner.Run(ctx, "systemctl", "restart", "xray.service")
	return err
}

func (m Manager) Render(cfg stack.Config) ([]byte, error) {
	return json.MarshalIndent(Generate(cfg), "", "  ")
}

func (m Manager) Validate(ctx context.Context, cfg stack.Config, rendered []byte) error {
	tmp, err := os.CreateTemp("", "zeroone-*.json")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
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
	res, err := runner.Run(ctx, resolveBinary(cfg), "run", "-test", "-config", tmp.Name())
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
	if err := activeRestarter.Restart(ctx, m.Runner); err != nil {
		return plan, fmt.Errorf("restart xray: %w", err)
	}
	return plan, nil
}
