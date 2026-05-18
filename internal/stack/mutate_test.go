package stack

import "testing"

func TestNormalizeDomainRule(t *testing.T) {
	cases := map[string]string{
		"https://geroogo.com/":      "geroogo.com",
		"http://Foo.example.com":    "foo.example.com",
		"  example.com  ":           "example.com",
		"example.com:8080":          "example.com",
		"example.com/some/path":     "example.com",
		"example.com?q=1":           "example.com",
		"sub.example.com":           "sub.example.com",
		"domain:example.com":        "domain:example.com",
		"  full:example.com":        "full:example.com",
		"regexp:.*\\.example\\.com": "regexp:.*\\.example\\.com",
		"geosite:category-ir":       "geosite:category-ir",
	}
	for in, want := range cases {
		got, err := NormalizeDomainRule(in)
		if err != nil {
			t.Errorf("NormalizeDomainRule(%q) error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizeDomainRule(%q) = %q want %q", in, got, want)
		}
	}
	for _, bad := range []string{"", "  ", "domain:", "https://", "..", "single"} {
		if _, err := NormalizeDomainRule(bad); err == nil {
			t.Errorf("NormalizeDomainRule(%q) should have failed", bad)
		}
	}
}

func baseTestConfig() Config {
	return Config{
		Server: ServerConfig{AdminListen: "127.0.0.1:1", XrayConfigPath: "/tmp/xray.json"},
		Xray: XrayConfig{
			Inbounds:  InboundConfig{VLESSWSPort: 443, VLESSXHTTPPort: 3002, LocalSOCKSPort: 10808, PublicSOCKS: []SOCKSInbound{{Name: "s", Port: 21000, Username: "u", Password: "p"}}},
			Outbounds: OutboundSet{Proxy: Outbound{Tag: "proxy"}},
			Users:     []User{{Email: "amir", UUID: "uuid", Enabled: true}},
		},
	}
}

func TestSetUserBandwidthAllocatesPort(t *testing.T) {
	cfg := baseTestConfig()
	if err := cfg.SetUserBandwidth("amir", 20, 5); err != nil {
		t.Fatal(err)
	}
	if cfg.Xray.Users[0].BandwidthPort != 21001 {
		t.Fatalf("unexpected bandwidth port: %d", cfg.Xray.Users[0].BandwidthPort)
	}
	if err := cfg.SetUserBandwidth("amir", 0, 0); err != nil {
		t.Fatal(err)
	}
	if cfg.Xray.Users[0].BandwidthPort != 0 {
		t.Fatalf("bandwidth port should be cleared: %d", cfg.Xray.Users[0].BandwidthPort)
	}
}

func TestUpsertClientEndpointDefaultsAndValidates(t *testing.T) {
	cfg := baseTestConfig()
	if err := cfg.UpsertClientEndpoint(ClientEndpoint{Name: "pars-pack", Host: "EDGE.EXAMPLE.COM", TLS: true, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	got := cfg.Server.ClientEndpoints[0]
	if got.Host != "edge.example.com" || got.Port != 443 || got.Network != "ws" || got.Path != "/vless" {
		t.Fatalf("unexpected endpoint defaults: %+v", got)
	}
	if err := cfg.UpsertClientEndpoint(ClientEndpoint{Name: "pars-pack", Host: "edge.example.com", Port: 443, Network: "tcp", Path: "/vless", TLS: true}); err == nil {
		t.Fatal("invalid network should fail")
	}
	if err := cfg.DeleteClientEndpoint("pars-pack"); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Server.ClientEndpoints) != 0 {
		t.Fatalf("endpoint should be deleted: %+v", cfg.Server.ClientEndpoints)
	}
}
