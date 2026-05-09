package xray

import (
	"testing"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
)

func TestGenerateAddsLimitedInbound(t *testing.T) {
	cfg := stack.Config{Xray: stack.XrayConfig{
		LogLevel: "warning",
		Inbounds: stack.InboundConfig{
			VLESSWSPort:    443,
			VLESSXHTTPPort: 3002,
			LocalSOCKSPort: 10808,
		},
		Users: []stack.User{{Email: "amir", UUID: "uuid", Enabled: true, DownloadMbps: 10, BandwidthPort: 21000}},
		Outbounds: stack.OutboundSet{
			Proxy:    stack.Outbound{Tag: "proxy", Type: "vless-ws", Address: "127.0.0.1", Port: 80, UUID: "uuid", Host: "host", Path: "/"},
			Fallback: stack.Outbound{Tag: "priority-proxy", Type: "vless-ws", Address: "127.0.0.1", Port: 80, UUID: "uuid", Host: "host", Path: "/"},
		},
	}}
	generated := Generate(cfg)
	inbounds := generated["inbounds"].([]Object)
	found := false
	for _, inbound := range inbounds {
		if inbound["tag"] == "limited-amir" {
			found = true
			if inbound["port"] != 21000 {
				t.Fatalf("unexpected port: %+v", inbound)
			}
		}
	}
	if !found {
		t.Fatalf("limited inbound not generated: %+v", inbounds)
	}
}
