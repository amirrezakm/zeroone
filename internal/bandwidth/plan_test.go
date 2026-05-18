package bandwidth

import (
	"strings"
	"testing"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
)

func TestActiveLimits(t *testing.T) {
	cfg := stack.Config{Xray: stack.XrayConfig{Users: []stack.User{
		{Email: "b", Enabled: true, BandwidthPort: 21002, DownloadMbps: 10},
		{Email: "a", Enabled: true, BandwidthPort: 21001, UploadMbps: 2},
		{Email: "disabled", Enabled: false, BandwidthPort: 21003, DownloadMbps: 10},
		{Email: "empty", Enabled: true, BandwidthPort: 21004},
	}}}
	limits := ActiveLimits(cfg)
	if len(limits) != 2 || limits[0].Email != "a" || limits[1].Email != "b" {
		t.Fatalf("unexpected limits: %+v", limits)
	}
}

func TestNFTAndTCPlan(t *testing.T) {
	limits := []Limit{{Email: "amir", Port: 21000, DownloadMbps: 20, UploadMbps: 5}}
	nft := NFTScript(limits)
	if !strings.Contains(nft, "tcp sport 21000 meta mark set 41001") {
		t.Fatalf("missing nft mark: %s", nft)
	}
	commands := TCCommands("eth0", limits)
	joined := strings.Join(commands, "\n")
	if !strings.Contains(joined, "rate 20mbit") || !strings.Contains(joined, "dst_port 21000") {
		t.Fatalf("missing tc limit commands:\n%s", joined)
	}
}
