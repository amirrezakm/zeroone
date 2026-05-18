package xray

import (
	"testing"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
)

func baseCfg() stack.Config {
	return stack.Config{Xray: stack.XrayConfig{
		LogLevel: "warning",
		Inbounds: stack.InboundConfig{
			VLESSWSPort:    443,
			VLESSXHTTPPort: 3002,
			LocalSOCKSPort: 10808,
		},
		Outbounds: stack.OutboundSet{
			Proxy:    stack.Outbound{Tag: "proxy", Type: "vless-ws", Address: "127.0.0.1", Port: 80, UUID: "u", Host: "h", Path: "/"},
			Fallback: stack.Outbound{Tag: "priority-proxy", Type: "vless-ws", Address: "127.0.0.1", Port: 80, UUID: "u", Host: "h", Path: "/"},
		},
	}}
}

// hasOutbound returns true if an outbound with the given tag is present.
func hasOutbound(obj Object, tag string) bool {
	for _, o := range obj["outbounds"].([]Object) {
		if o["tag"] == tag {
			return true
		}
	}
	return false
}

// findRule returns the first routing rule with outboundTag == tag.
func findRule(obj Object, tag string) Object {
	for _, r := range obj["routing"].(Object)["rules"].([]Object) {
		if r["outboundTag"] == tag {
			return r
		}
	}
	return nil
}

func TestRelayDisabledOmitsOutbound(t *testing.T) {
	cfg := baseCfg()
	cfg.Relay = stack.RelayConfig{Enabled: false, Sites: []stack.RelaySite{{Domain: "x.com", Enabled: true}}}
	gen := Generate(cfg)
	if hasOutbound(gen, stack.DefaultRelayOutboundTag) {
		t.Fatalf("relay outbound should not appear when disabled")
	}
	if r := findRule(gen, stack.DefaultRelayOutboundTag); r != nil {
		t.Fatalf("relay routing rule should not appear when disabled: %+v", r)
	}
}

func TestRelayEnabledWithNoSitesSkipsRule(t *testing.T) {
	cfg := baseCfg()
	cfg.Relay = stack.RelayConfig{Enabled: true} // no sites
	gen := Generate(cfg)
	if hasOutbound(gen, stack.DefaultRelayOutboundTag) {
		t.Fatalf("relay outbound should not appear without enabled sites")
	}
}

func TestRelayEnabledEmitsOutboundAndRule(t *testing.T) {
	cfg := baseCfg()
	cfg.Relay = stack.RelayConfig{
		Enabled:     true,
		Listen:      "127.0.0.1:8085",
		OutboundTag: "relay-mhrv",
		Sites: []stack.RelaySite{
			{Domain: "youtube.com", Enabled: true},
			{Domain: "blocked.example", Enabled: false}, // disabled, must be excluded
			{Domain: "domain:openai.com", Enabled: true},
		},
	}
	gen := Generate(cfg)
	if !hasOutbound(gen, "relay-mhrv") {
		t.Fatalf("relay outbound missing")
	}
	rule := findRule(gen, "relay-mhrv")
	if rule == nil {
		t.Fatalf("relay routing rule missing")
	}
	domains, ok := rule["domain"].([]string)
	if !ok {
		t.Fatalf("rule domain type unexpected: %T", rule["domain"])
	}
	if len(domains) != 2 {
		t.Fatalf("expected 2 domains, got %v", domains)
	}
	got := map[string]bool{}
	for _, d := range domains {
		got[d] = true
	}
	if !got["youtube.com"] || !got["domain:openai.com"] {
		t.Fatalf("missing expected domains: %v", domains)
	}
	if got["blocked.example"] {
		t.Fatalf("disabled site should not be in routing rule: %v", domains)
	}
}

func TestRelayInboundTagsRoutesByInbound(t *testing.T) {
	cfg := baseCfg()
	cfg.Relay = stack.RelayConfig{
		Enabled:     true,
		OutboundTag: "relay-mhrv",
		InboundTags: []string{"relay-socks"},
		// intentionally NO sites — verifying inbound-only routing still
		// emits the outbound and routing rule.
	}
	gen := Generate(cfg)
	if !hasOutbound(gen, "relay-mhrv") {
		t.Fatalf("inbound-only relay should still emit outbound")
	}
	rule := findRule(gen, "relay-mhrv")
	if rule == nil {
		t.Fatalf("inbound-tag routing rule missing")
	}
	inbounds, ok := rule["inboundTag"].([]string)
	if !ok || len(inbounds) != 1 || inbounds[0] != "relay-socks" {
		t.Fatalf("expected inboundTag=[relay-socks], got %v", rule["inboundTag"])
	}
	if _, hasDomain := rule["domain"]; hasDomain {
		t.Fatalf("inbound rule should not carry a domain matcher: %+v", rule)
	}
}

func TestRelayCustomTag(t *testing.T) {
	cfg := baseCfg()
	cfg.Relay = stack.RelayConfig{
		Enabled:     true,
		OutboundTag: "my-relay",
		Sites:       []stack.RelaySite{{Domain: "foo.com", Enabled: true}},
	}
	gen := Generate(cfg)
	if !hasOutbound(gen, "my-relay") {
		t.Fatalf("custom-tag outbound missing")
	}
	if findRule(gen, "my-relay") == nil {
		t.Fatalf("custom-tag routing rule missing")
	}
}
