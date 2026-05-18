package links

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
)

func TestVLESSIncludesConfiguredClientEndpoint(t *testing.T) {
	cfg := stack.Config{
		Server: stack.ServerConfig{
			PublicIP: "185.128.139.68",
			ClientEndpoints: []stack.ClientEndpoint{{
				Name:    "pars-pack",
				Host:    "edge.payanekhosh.ir",
				Port:    443,
				Network: "xhttp",
				Path:    "/api/v1/events",
				Mode:    "stream-up",
				TLS:     true,
				Enabled: true,
			}},
		},
		Xray: stack.XrayConfig{Inbounds: stack.InboundConfig{VLESSWSPort: 443, VLESSXHTTPPort: 3002, VLESSXHTTPMode: "stream-up"}},
	}
	got := VLESS(cfg, stack.User{Email: "ali", UUID: "uuid", Enabled: true})
	if len(got) != 3 {
		t.Fatalf("expected legacy ws, legacy xhttp, and CDN links, got %d", len(got))
	}
	cdn := got[2]
	if cdn.Name != "ZeroOne · Pars Pack" {
		t.Fatalf("unexpected CDN link name: %s", cdn.Name)
	}
	for _, want := range []string{"edge.payanekhosh.ir:443", "security=tls", "sni=edge.payanekhosh.ir", "host=edge.payanekhosh.ir", "path=%2Fapi%2Fv1%2Fevents", "type=xhttp", "mode=stream-up"} {
		if !strings.Contains(cdn.URL, want) {
			t.Fatalf("CDN URL missing %q: %s", want, cdn.URL)
		}
	}
	if strings.Contains(got[0].URL, "edge.payanekhosh.ir") {
		t.Fatalf("legacy link should remain on public IP: %s", got[0].URL)
	}
}

func TestVLESSEmitsXHTTPExtra(t *testing.T) {
	cfg := stack.Config{
		Server: stack.ServerConfig{
			PublicIP: "185.128.139.68",
			ClientEndpoints: []stack.ClientEndpoint{{
				Name:    "pars-pack",
				Host:    "edge.payanekhosh.ir",
				Port:    443,
				Network: "xhttp",
				Path:    "/api/v1/events",
				Mode:    "stream-up",
				TLS:     true,
				Enabled: true,
				Extra: stack.XHTTPClientExtra{
					XPaddingBytes: "100-1000",
					Xmux: &stack.XmuxSettings{
						MaxConcurrency: "16-24",
						CMaxLifetimeMs: 90000,
					},
				},
			}},
		},
		Xray: stack.XrayConfig{Inbounds: stack.InboundConfig{VLESSWSPort: 443, VLESSXHTTPPort: 3002, VLESSXHTTPMode: "auto"}},
	}
	got := VLESS(cfg, stack.User{Email: "ali", UUID: "uuid", Enabled: true})
	cdn := got[len(got)-1]
	u, err := url.Parse(cdn.URL)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	raw := u.Query().Get("extra")
	if raw == "" {
		t.Fatalf("expected extra= in URL, got %s", cdn.URL)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("extra not valid JSON: %v (%q)", err, raw)
	}
	if parsed["xPaddingBytes"] != "100-1000" {
		t.Errorf("xPaddingBytes = %v", parsed["xPaddingBytes"])
	}
	mux, ok := parsed["xmux"].(map[string]any)
	if !ok {
		t.Fatalf("xmux missing or wrong type: %#v", parsed["xmux"])
	}
	if mux["maxConcurrency"] != "16-24" {
		t.Errorf("xmux.maxConcurrency = %v", mux["maxConcurrency"])
	}
	if mux["cMaxLifetimeMs"] != float64(90000) {
		t.Errorf("xmux.cMaxLifetimeMs = %v", mux["cMaxLifetimeMs"])
	}
	// legacy direct xhttp link must not carry extra= because the local
	// inbound has no client-side tuning configured.
	for _, l := range got[:len(got)-1] {
		if strings.Contains(l.URL, "extra=") {
			t.Errorf("non-CDN link unexpectedly carries extra=: %s", l.URL)
		}
	}
}

func TestVLESSEmitsSplitHTTPCompatLinks(t *testing.T) {
	cfg := stack.Config{
		Server: stack.ServerConfig{
			PublicIP: "185.128.139.68",
			ClientEndpoints: []stack.ClientEndpoint{{
				Name:       "pars-pack",
				Host:       "edge.payanekhosh.ir",
				Port:       443,
				Network:    "xhttp",
				Path:       "/api/v1/events",
				Mode:       "stream-up",
				TLS:        true,
				Enabled:    true,
				LinkCompat: true,
				Extra: stack.XHTTPClientExtra{
					XPaddingBytes: "100-1000",
				},
			}},
		},
		Xray: stack.XrayConfig{Inbounds: stack.InboundConfig{
			VLESSWSPort:          443,
			VLESSXHTTPPort:       3002,
			VLESSXHTTPPath:       "/api/v1/events",
			VLESSXHTTPMode:       "stream-up",
			VLESSXHTTPLinkCompat: true,
		}},
	}
	got := VLESS(cfg, stack.User{Email: "ali", UUID: "uuid", Enabled: true})
	// expected order: ws, xhttp (local), splithttp (local compat), xhttp (cdn), splithttp (cdn compat)
	if len(got) != 5 {
		t.Fatalf("expected 5 links, got %d: %+v", len(got), got)
	}

	localCompat := got[2]
	if localCompat.Name != "ZeroOne · Direct · SplitHTTP" {
		t.Fatalf("unexpected local compat name: %s", localCompat.Name)
	}
	for _, want := range []string{"type=splithttp", "host=185.128.139.68", "security=none", "path=%2Fapi%2Fv1%2Fevents", "mode=stream-up"} {
		if !strings.Contains(localCompat.URL, want) {
			t.Errorf("local compat URL missing %q: %s", want, localCompat.URL)
		}
	}
	if strings.Contains(localCompat.URL, "extra=") {
		t.Errorf("compat link must not carry extra=: %s", localCompat.URL)
	}

	cdnCompat := got[4]
	if cdnCompat.Name != "ZeroOne · Pars Pack · SplitHTTP" {
		t.Fatalf("unexpected CDN compat name: %s", cdnCompat.Name)
	}
	for _, want := range []string{"type=splithttp", "host=edge.payanekhosh.ir", "security=tls", "sni=edge.payanekhosh.ir", "mode=stream-up"} {
		if !strings.Contains(cdnCompat.URL, want) {
			t.Errorf("CDN compat URL missing %q: %s", want, cdnCompat.URL)
		}
	}
	if strings.Contains(cdnCompat.URL, "extra=") {
		t.Errorf("compat link must not carry extra=: %s", cdnCompat.URL)
	}
	// non-compat xhttp links must still carry the xray-native type.
	for _, l := range []Link{got[1], got[3]} {
		if !strings.Contains(l.URL, "type=xhttp") {
			t.Errorf("native xhttp link should retain type=xhttp: %s", l.URL)
		}
	}
}
