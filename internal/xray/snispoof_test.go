package xray

import (
	"testing"

	"github.com/amirrezakm/zeroone/internal/stack"
)

func TestSNISpoofDisabledOmitsOutbound(t *testing.T) {
	cfg := baseCfg()
	cfg.SNISpoof = stack.SNISpoofConfig{Enabled: false, Sites: []stack.RelaySite{{Domain: "youtube.com", Enabled: true}}}
	gen := Generate(cfg)
	if hasOutbound(gen, stack.DefaultSNISpoofOutboundTag) {
		t.Fatalf("sni-spoof outbound should not appear when disabled")
	}
	if r := findRule(gen, stack.DefaultSNISpoofOutboundTag); r != nil {
		t.Fatalf("sni-spoof routing rule should not appear when disabled: %+v", r)
	}
}

func TestSNISpoofEnabledNoSitesSkipsOutbound(t *testing.T) {
	cfg := baseCfg()
	cfg.SNISpoof = stack.SNISpoofConfig{Enabled: true} // no sites, no inbound tags
	gen := Generate(cfg)
	if hasOutbound(gen, stack.DefaultSNISpoofOutboundTag) {
		t.Fatalf("sni-spoof outbound should not appear without targets")
	}
}

func TestSNISpoofEnabledEmitsMarkedOutboundAndRule(t *testing.T) {
	cfg := baseCfg()
	cfg.SNISpoof = stack.SNISpoofConfig{
		Enabled:      true,
		FakeDomain:   "www.hcaptcha.com",
		Method:       "fake",
		FirewallMark: 7137,
		Sites: []stack.RelaySite{
			{Domain: "youtube.com", Enabled: true},
			{Domain: "off.example", Enabled: false}, // excluded
		},
	}
	gen := Generate(cfg)
	if !hasOutbound(gen, stack.DefaultSNISpoofOutboundTag) {
		t.Fatalf("sni-spoof outbound missing")
	}
	// The outbound must stamp SO_MARK so the policy route can divert it.
	var ob Object
	for _, o := range gen["outbounds"].([]Object) {
		if o["tag"] == stack.DefaultSNISpoofOutboundTag {
			ob = o
		}
	}
	if ob["protocol"] != "freedom" {
		t.Fatalf("expected freedom outbound, got %v", ob["protocol"])
	}
	sockopt := ob["streamSettings"].(Object)["sockopt"].(Object)
	if sockopt["mark"] != 7137 {
		t.Fatalf("expected sockopt mark 7137, got %v", sockopt["mark"])
	}
	rule := findRule(gen, stack.DefaultSNISpoofOutboundTag)
	if rule == nil {
		t.Fatalf("sni-spoof routing rule missing")
	}
	domains, ok := rule["domain"].([]string)
	if !ok || len(domains) != 1 || domains[0] != "youtube.com" {
		t.Fatalf("expected domain [youtube.com], got %v", rule["domain"])
	}
}

func TestSNISpoofDefaultMarkApplied(t *testing.T) {
	cfg := baseCfg()
	cfg.SNISpoof = stack.SNISpoofConfig{
		Enabled: true,
		Sites:   []stack.RelaySite{{Domain: "x.com", Enabled: true}},
	}
	gen := Generate(cfg)
	for _, o := range gen["outbounds"].([]Object) {
		if o["tag"] == stack.DefaultSNISpoofOutboundTag {
			sockopt := o["streamSettings"].(Object)["sockopt"].(Object)
			if sockopt["mark"] != stack.DefaultSNISpoofFirewallMark {
				t.Fatalf("expected default mark %d, got %v", stack.DefaultSNISpoofFirewallMark, sockopt["mark"])
			}
			return
		}
	}
	t.Fatalf("sni-spoof outbound missing")
}
