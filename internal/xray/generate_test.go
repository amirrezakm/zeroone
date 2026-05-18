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

func TestGenerateAppliesXHTTPTuning(t *testing.T) {
	noSSE := false
	cfg := stack.Config{Xray: stack.XrayConfig{
		LogLevel: "warning",
		Inbounds: stack.InboundConfig{
			VLESSWSPort:    443,
			VLESSXHTTPPort: 3002,
			VLESSXHTTPPath: "/api/v1/events",
			VLESSXHTTPMode: "auto",
			VLESSXHTTPTuning: stack.XHTTPInboundTuning{
				XPaddingBytes:        "100-1000",
				ScMaxBufferedPosts:   30,
				ScMaxEachPostBytes:   "1000000",
				ScStreamUpServerSecs: "20-80",
				KeepAlivePeriod:      30,
				NoSSEHeader:          &noSSE,
			},
			LocalSOCKSPort: 10808,
		},
		Users: []stack.User{{Email: "amir", UUID: "uuid", Enabled: true}},
		Outbounds: stack.OutboundSet{
			Proxy:    stack.Outbound{Tag: "proxy", Type: "vless-ws", Address: "127.0.0.1", Port: 80, UUID: "uuid", Host: "host", Path: "/"},
			Fallback: stack.Outbound{Tag: "priority-proxy", Type: "vless-ws", Address: "127.0.0.1", Port: 80, UUID: "uuid", Host: "host", Path: "/"},
		},
	}}
	generated := Generate(cfg)
	var settings Object
	for _, inbound := range generated["inbounds"].([]Object) {
		if inbound["tag"] == "vless-xhttp-local" {
			settings = inbound["streamSettings"].(Object)["xhttpSettings"].(Object)
			break
		}
	}
	if settings == nil {
		t.Fatalf("xhttp inbound missing")
	}
	want := map[string]any{
		"xPaddingBytes":        "100-1000",
		"scMaxBufferedPosts":   30,
		"scMaxEachPostBytes":   1000000,
		"scStreamUpServerSecs": "20-80",
		"keepAlivePeriod":      30,
		"noSSEHeader":          false,
	}
	for k, v := range want {
		if got := settings[k]; got != v {
			t.Errorf("xhttpSettings[%q] = %v (%T), want %v (%T)", k, got, got, v, v)
		}
	}
}

func TestGenerateUsesConfiguredXHTTPPath(t *testing.T) {
	cfg := stack.Config{Xray: stack.XrayConfig{
		LogLevel: "warning",
		Inbounds: stack.InboundConfig{
			VLESSWSPort:    443,
			VLESSXHTTPPort: 3002,
			VLESSXHTTPPath: "/api/v1/events",
			VLESSXHTTPMode: "stream-up",
			LocalSOCKSPort: 10808,
		},
		Users: []stack.User{{Email: "amir", UUID: "uuid", Enabled: true}},
		Outbounds: stack.OutboundSet{
			Proxy:    stack.Outbound{Tag: "proxy", Type: "vless-ws", Address: "127.0.0.1", Port: 80, UUID: "uuid", Host: "host", Path: "/"},
			Fallback: stack.Outbound{Tag: "priority-proxy", Type: "vless-ws", Address: "127.0.0.1", Port: 80, UUID: "uuid", Host: "host", Path: "/"},
		},
	}}
	generated := Generate(cfg)
	for _, inbound := range generated["inbounds"].([]Object) {
		if inbound["tag"] != "vless-xhttp-local" {
			continue
		}
		stream := inbound["streamSettings"].(Object)
		settings := stream["xhttpSettings"].(Object)
		if settings["path"] != "/api/v1/events" {
			t.Fatalf("unexpected xhttp path: %+v", settings)
		}
		if settings["mode"] != "stream-up" {
			t.Fatalf("unexpected xhttp mode: %+v", settings)
		}
		return
	}
	t.Fatalf("xhttp inbound not generated: %+v", generated["inbounds"])
}
