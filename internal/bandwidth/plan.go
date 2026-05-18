package bandwidth

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
)

const (
	defaultDevice = "eth0"
	baseMark      = 41000
	baseClass     = 100
	defaultRate   = "1000mbit"
)

type Limit struct {
	Email        string `json:"email"`
	Port         int    `json:"port"`
	DownloadMbps int    `json:"download_mbps"`
	UploadMbps   int    `json:"upload_mbps"`
}

type Plan struct {
	Device      string   `json:"device"`
	Limits      []Limit  `json:"limits"`
	NFTScript   string   `json:"nft_script"`
	TCCommands  []string `json:"tc_commands"`
	NeedsApply  bool     `json:"needs_apply"`
	ApplyLocked bool     `json:"apply_locked"`
}

func BuildPlan(cfg stack.Config, allowApply bool) Plan {
	device := cfg.Server.BandwidthDevice
	if device == "" {
		device = defaultDevice
	}
	limits := ActiveLimits(cfg)
	return Plan{
		Device:      device,
		Limits:      limits,
		NFTScript:   NFTScript(limits),
		TCCommands:  TCCommands(device, limits),
		NeedsApply:  len(limits) > 0,
		ApplyLocked: !allowApply,
	}
}

func ActiveLimits(cfg stack.Config) []Limit {
	limits := []Limit{}
	for _, u := range cfg.Xray.Users {
		if !u.Enabled || u.BandwidthPort <= 0 || (u.DownloadMbps <= 0 && u.UploadMbps <= 0) {
			continue
		}
		limits = append(limits, Limit{
			Email:        u.Email,
			Port:         u.BandwidthPort,
			DownloadMbps: u.DownloadMbps,
			UploadMbps:   u.UploadMbps,
		})
	}
	sort.Slice(limits, func(i, j int) bool { return limits[i].Email < limits[j].Email })
	return limits
}

func NFTScript(limits []Limit) string {
	var b strings.Builder
	b.WriteString("table inet xray_bw {\n")
	b.WriteString("  chain output {\n")
	b.WriteString("    type route hook output priority mangle; policy accept;\n")
	for i, limit := range limits {
		_, _ = fmt.Fprintf(&b, "    tcp sport %d meta mark set %d\n", limit.Port, baseMark+i+1)
	}
	b.WriteString("  }\n")
	b.WriteString("}\n")
	return b.String()
}

func TCCommands(device string, limits []Limit) []string {
	if len(limits) == 0 {
		return []string{
			fmt.Sprintf("nft delete table inet xray_bw"),
			fmt.Sprintf("tc qdisc del dev %s root", device),
			fmt.Sprintf("tc qdisc del dev %s clsact", device),
			fmt.Sprintf("tc qdisc replace dev %s root fq", device),
		}
	}
	commands := []string{
		fmt.Sprintf("nft -f <generated xray_bw script>"),
		fmt.Sprintf("tc qdisc del dev %s root", device),
		fmt.Sprintf("tc qdisc del dev %s clsact", device),
		fmt.Sprintf("tc qdisc add dev %s root handle 1: htb default 999", device),
		fmt.Sprintf("tc class replace dev %s parent 1: classid 1:1 htb rate %s ceil %s", device, defaultRate, defaultRate),
		fmt.Sprintf("tc class replace dev %s parent 1:1 classid 1:999 htb rate %s ceil %s", device, defaultRate, defaultRate),
		fmt.Sprintf("tc qdisc replace dev %s parent 1:999 fq", device),
		fmt.Sprintf("tc qdisc add dev %s clsact", device),
	}
	for i, limit := range limits {
		mark := baseMark + i + 1
		classID := fmt.Sprintf("1:%d", baseClass+i+1)
		if limit.DownloadMbps > 0 {
			rate := fmt.Sprintf("%dmbit", limit.DownloadMbps)
			commands = append(commands,
				fmt.Sprintf("tc class replace dev %s parent 1:1 classid %s htb rate %s ceil %s", device, classID, rate, rate),
				fmt.Sprintf("tc qdisc replace dev %s parent %s fq", device, classID),
				fmt.Sprintf("tc filter replace dev %s parent 1: protocol ip prio %d handle %d fw flowid %s", device, 100+i+1, mark, classID),
			)
		}
		if limit.UploadMbps > 0 {
			rate := fmt.Sprintf("%dmbit", limit.UploadMbps)
			burst := max(64, limit.UploadMbps*128)
			commands = append(commands,
				fmt.Sprintf("tc filter replace dev %s ingress protocol ip prio %d flower ip_proto tcp dst_port %d action police rate %s burst %dk conform-exceed drop", device, 100+i+1, limit.Port, rate, burst),
			)
		}
	}
	return commands
}
