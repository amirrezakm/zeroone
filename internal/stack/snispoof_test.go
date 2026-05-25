package stack

import "testing"

// validBase returns a minimal Config that passes Validate(), so snispoof
// cases only exercise the sni_spoof-specific rules.
func validBase() Config {
	return Config{
		Server: ServerConfig{AdminListen: "127.0.0.1:8080", XrayConfigPath: "/tmp/xray.json"},
		Xray: XrayConfig{
			Inbounds:  InboundConfig{VLESSWSPort: 443},
			Outbounds: OutboundSet{Proxy: Outbound{Tag: "proxy"}},
		},
	}
}

func TestSNISpoofValidateOK(t *testing.T) {
	cfg := validBase()
	cfg.SNISpoof = SNISpoofConfig{
		Enabled:    true,
		FakeDomain: "www.hcaptcha.com",
		Sites:      []RelaySite{{Domain: "youtube.com", Enabled: true}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
}

func TestSNISpoofValidateRejectsTagConflict(t *testing.T) {
	cfg := validBase()
	cfg.SNISpoof = SNISpoofConfig{Enabled: true, OutboundTag: "direct"}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for built-in tag conflict")
	}
}

func TestSNISpoofValidateRejectsBadMethod(t *testing.T) {
	cfg := validBase()
	cfg.SNISpoof = SNISpoofConfig{Enabled: true, Method: "wormhole"}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for unknown method")
	}
}

func TestSNISpoofValidateRejectsBadTunAddr(t *testing.T) {
	cfg := validBase()
	cfg.SNISpoof = SNISpoofConfig{Enabled: true, TunAddr: "not-a-cidr"}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for bad tun_addr")
	}
}

func TestSNISpoofValidateRejectsDuplicateSites(t *testing.T) {
	cfg := validBase()
	cfg.SNISpoof = SNISpoofConfig{
		Enabled: true,
		Sites:   []RelaySite{{Domain: "a.com"}, {Domain: "a.com"}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for duplicate sites")
	}
}

func TestSNISpoofValidateRejectsPortClashWithInbound(t *testing.T) {
	cfg := validBase() // vless-ws is on 443
	cfg.SNISpoof = SNISpoofConfig{Enabled: true, Listen: "127.0.0.1:443"}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for listen port collision with vless-ws inbound")
	}
}

func TestSNISpoofDisabledSkipsValidation(t *testing.T) {
	cfg := validBase()
	// Garbage values are tolerated while disabled.
	cfg.SNISpoof = SNISpoofConfig{Enabled: false, Method: "bogus", TunAddr: "xxx", OutboundTag: "direct"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("disabled snispoof should not be validated, got %v", err)
	}
}
