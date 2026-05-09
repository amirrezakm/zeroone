package system

import (
	"context"
	"os/exec"
	"time"
)

type Result struct {
	Stdout string
	Stderr string
}

type Runner interface {
	Run(ctx context.Context, name string, args ...string) (Result, error)
}

type ExecRunner struct{ Timeout time.Duration }

func (r ExecRunner) Run(ctx context.Context, name string, args ...string) (Result, error) {
	if r.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err := cmd.Output()
	res := Result{Stdout: string(stdout)}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			res.Stderr = string(ee.Stderr)
		}
		return res, err
	}
	return res, nil
}
