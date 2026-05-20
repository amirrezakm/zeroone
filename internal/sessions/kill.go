package sessions

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/amirrezakm/zeroone/internal/system"
)

type KillResult struct {
	Email  string   `json:"email"`
	Killed int      `json:"killed"`
	Ports  []int    `json:"ports"`
	IPs    []string `json:"ips"`
}

func KillByPeerIPs(ctx context.Context, runner system.Runner, ports []int, ips []string) (KillResult, error) {
	if runner == nil {
		runner = system.ExecRunner{Timeout: 6 * time.Second}
	}
	out := KillResult{Ports: ports, IPs: ips}
	if len(ports) == 0 || len(ips) == 0 {
		return out, nil
	}
	for _, port := range ports {
		for _, ip := range ips {
			filter := fmt.Sprintf("( sport = :%d dst %s )", port, ip)
			res, err := runner.Run(ctx, "ss", "-ntKH", "state", "established", filter)
			if err != nil {
				continue
			}
			for _, line := range strings.Split(res.Stdout, "\n") {
				if strings.TrimSpace(line) != "" {
					out.Killed++
				}
			}
		}
	}
	return out, nil
}
