package bandwidth

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
	"github.com/sakhtar/xray-stack-zeroone/internal/system"
)

type ApplyResult struct {
	Device   string   `json:"device"`
	Limits   []Limit  `json:"limits"`
	Commands []string `json:"commands"`
	Applied  bool     `json:"applied"`
}

type Manager struct {
	Runner system.Runner
}

type command struct {
	name        string
	args        []string
	description string
	ignoreError bool
}

func (m Manager) Apply(ctx context.Context, cfg stack.Config) (ApplyResult, error) {
	device := cfg.Server.BandwidthDevice
	if device == "" {
		device = defaultDevice
	}
	limits := ActiveLimits(cfg)
	result := ApplyResult{Device: device, Limits: limits}
	runner := m.Runner
	if runner == nil {
		runner = system.ExecRunner{Timeout: 15 * time.Second}
	}
	commands, cleanup, err := applyCommands(device, limits)
	if err != nil {
		return result, err
	}
	defer cleanup()
	for _, cmd := range commands {
		result.Commands = append(result.Commands, cmd.description)
		res, err := runner.Run(ctx, cmd.name, cmd.args...)
		if err != nil && !cmd.ignoreError {
			return result, fmt.Errorf("%s failed: %w: %s%s", cmd.description, err, res.Stdout, res.Stderr)
		}
	}
	result.Applied = true
	return result, nil
}

func applyCommands(device string, limits []Limit) ([]command, func(), error) {
	cleanup := func() {}
	commands := []command{
		{name: "nft", args: []string{"delete", "table", "inet", "xray_bw"}, description: "nft delete table inet xray_bw", ignoreError: true},
	}
	if len(limits) == 0 {
		commands = append(commands,
			command{name: "tc", args: []string{"qdisc", "del", "dev", device, "root"}, description: fmt.Sprintf("tc qdisc del dev %s root", device), ignoreError: true},
			command{name: "tc", args: []string{"qdisc", "del", "dev", device, "clsact"}, description: fmt.Sprintf("tc qdisc del dev %s clsact", device), ignoreError: true},
			command{name: "tc", args: []string{"qdisc", "replace", "dev", device, "root", "fq"}, description: fmt.Sprintf("tc qdisc replace dev %s root fq", device)},
		)
		return commands, cleanup, nil
	}
	tmp, err := os.CreateTemp("", "xray-bw-*.nft")
	if err != nil {
		return nil, cleanup, err
	}
	cleanup = func() { _ = os.Remove(tmp.Name()) }
	if _, err := tmp.WriteString(NFTScript(limits)); err != nil {
		_ = tmp.Close()
		cleanup()
		return nil, func() {}, err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return nil, func() {}, err
	}
	commands = append(commands,
		command{name: "nft", args: []string{"-f", tmp.Name()}, description: "nft -f <generated xray_bw script>"},
		command{name: "tc", args: []string{"qdisc", "replace", "dev", device, "root", "handle", "1:", "htb", "default", "999"}, description: fmt.Sprintf("tc qdisc replace dev %s root handle 1: htb default 999", device)},
		command{name: "tc", args: []string{"class", "replace", "dev", device, "parent", "1:", "classid", "1:1", "htb", "rate", defaultRate, "ceil", defaultRate}, description: fmt.Sprintf("tc class replace dev %s parent 1: classid 1:1 htb rate %s ceil %s", device, defaultRate, defaultRate)},
		command{name: "tc", args: []string{"class", "replace", "dev", device, "parent", "1:1", "classid", "1:999", "htb", "rate", defaultRate, "ceil", defaultRate}, description: fmt.Sprintf("tc class replace dev %s parent 1:1 classid 1:999 htb rate %s ceil %s", device, defaultRate, defaultRate)},
		command{name: "tc", args: []string{"qdisc", "replace", "dev", device, "parent", "1:999", "fq"}, description: fmt.Sprintf("tc qdisc replace dev %s parent 1:999 fq", device)},
		command{name: "tc", args: []string{"qdisc", "replace", "dev", device, "clsact"}, description: fmt.Sprintf("tc qdisc replace dev %s clsact", device)},
	)
	for i, limit := range limits {
		mark := fmt.Sprintf("%d", baseMark+i+1)
		classID := fmt.Sprintf("1:%d", baseClass+i+1)
		prio := fmt.Sprintf("%d", 100+i+1)
		if limit.DownloadMbps > 0 {
			rate := fmt.Sprintf("%dmbit", limit.DownloadMbps)
			commands = append(commands,
				command{name: "tc", args: []string{"class", "replace", "dev", device, "parent", "1:1", "classid", classID, "htb", "rate", rate, "ceil", rate}, description: fmt.Sprintf("tc class replace dev %s parent 1:1 classid %s htb rate %s ceil %s", device, classID, rate, rate)},
				command{name: "tc", args: []string{"qdisc", "replace", "dev", device, "parent", classID, "fq"}, description: fmt.Sprintf("tc qdisc replace dev %s parent %s fq", device, classID)},
				command{name: "tc", args: []string{"filter", "replace", "dev", device, "parent", "1:", "protocol", "ip", "prio", prio, "handle", mark, "fw", "flowid", classID}, description: fmt.Sprintf("tc filter replace dev %s parent 1: protocol ip prio %s handle %s fw flowid %s", device, prio, mark, classID)},
			)
		}
		if limit.UploadMbps > 0 {
			rate := fmt.Sprintf("%dmbit", limit.UploadMbps)
			burst := fmt.Sprintf("%dk", max(64, limit.UploadMbps*128))
			commands = append(commands,
				command{name: "tc", args: []string{"filter", "replace", "dev", device, "ingress", "protocol", "ip", "prio", prio, "flower", "ip_proto", "tcp", "dst_port", fmt.Sprintf("%d", limit.Port), "action", "police", "rate", rate, "burst", burst, "conform-exceed", "drop"}, description: fmt.Sprintf("tc filter replace dev %s ingress protocol ip prio %s flower ip_proto tcp dst_port %d action police rate %s burst %s conform-exceed drop", device, prio, limit.Port, rate, burst)},
			)
		}
	}
	return commands, cleanup, nil
}
