package firewall

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/system"
)

const ruleComment = "xray-stack bandwidth-limited"

type UFW struct {
	Runner system.Runner
}

func (u UFW) runner() system.Runner {
	if u.Runner != nil {
		return u.Runner
	}
	return system.ExecRunner{Timeout: 10 * time.Second}
}

func (u UFW) Allow(ctx context.Context, port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port %d", port)
	}
	if _, err := exec.LookPath("ufw"); err != nil {
		return nil
	}
	res, err := u.runner().Run(ctx, "ufw", "allow", fmt.Sprintf("%d/tcp", port), "comment", ruleComment)
	if err != nil {
		return fmt.Errorf("ufw allow %d/tcp: %w: %s%s", port, err, res.Stdout, res.Stderr)
	}
	return nil
}

func (u UFW) Delete(ctx context.Context, port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port %d", port)
	}
	if _, err := exec.LookPath("ufw"); err != nil {
		return nil
	}
	res, err := u.runner().Run(ctx, "ufw", "delete", "allow", fmt.Sprintf("%d/tcp", port))
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(res.Stderr)+len(res.Stdout) > 0 {
			combined := res.Stdout + res.Stderr
			if containsAny(combined, "Could not delete", "ERROR: Could not find") {
				return nil
			}
		}
		return fmt.Errorf("ufw delete allow %d/tcp: %w: %s%s", port, err, res.Stdout, res.Stderr)
	}
	return nil
}

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if len(n) > 0 && len(s) >= len(n) {
			for i := 0; i+len(n) <= len(s); i++ {
				if s[i:i+len(n)] == n {
					return true
				}
			}
		}
	}
	return false
}
