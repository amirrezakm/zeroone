package stack

import "testing"

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
