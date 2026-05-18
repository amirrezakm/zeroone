package subscription

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sakhtar/xray-stack-zeroone/internal/links"
	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
	"github.com/sakhtar/xray-stack-zeroone/internal/usage"
)

var sampleLinks = []links.Link{
	{Name: "amirreza-ws", URL: "vless://aaaa@1.2.3.4:443?type=ws&path=%2Fvless&security=none#amirreza-ws"},
	{Name: "amirreza-runflare-xhttp-auto", URL: "vless://aaaa@edge.example.com:443?type=xhttp&path=%2Fapi%2Fv1%2Fevents&security=tls&sni=edge.example.com&host=edge.example.com&mode=auto#amirreza-runflare-xhttp-auto"},
}

func TestEncodeBase64RoundTrips(t *testing.T) {
	body, ct := Encode(FormatBase64, sampleLinks)
	if !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("content-type: %q", ct)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(decoded)), "\n")
	if len(lines) != len(sampleLinks) {
		t.Fatalf("expected %d lines, got %d", len(sampleLinks), len(lines))
	}
	for i, l := range lines {
		if l != sampleLinks[i].URL {
			t.Fatalf("line %d: got %q want %q", i, l, sampleLinks[i].URL)
		}
	}
}

func TestEncodeSingBoxShape(t *testing.T) {
	body, ct := Encode(FormatSingBox, sampleLinks)
	if !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content-type: %q", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("not valid json: %v\n%s", err, body)
	}
	outs, _ := got["outbounds"].([]any)
	// Expect: 2 vless + 1 selector + 1 direct + 1 block = 5
	if len(outs) != len(sampleLinks)+3 {
		t.Fatalf("expected %d outbounds, got %d: %s", len(sampleLinks)+3, len(outs), body)
	}
	first, _ := outs[0].(map[string]any)
	if first["type"] != "vless" || first["tag"] != "amirreza-ws" {
		t.Fatalf("first outbound: %+v", first)
	}
	t1, _ := first["transport"].(map[string]any)
	if t1["type"] != "ws" || t1["path"] != "/vless" {
		t.Fatalf("ws transport: %+v", t1)
	}
	// xhttp variant
	second, _ := outs[1].(map[string]any)
	t2, _ := second["transport"].(map[string]any)
	if t2["type"] != "xhttp" || t2["mode"] != "auto" {
		t.Fatalf("xhttp transport: %+v", t2)
	}
	tls2, _ := second["tls"].(map[string]any)
	if tls2 == nil || tls2["server_name"] != "edge.example.com" {
		t.Fatalf("xhttp tls block: %+v", tls2)
	}
}

func TestEncodeClashHasProxiesAndGroup(t *testing.T) {
	body, ct := Encode(FormatClash, sampleLinks)
	if !strings.Contains(ct, "yaml") {
		t.Fatalf("content-type: %q", ct)
	}
	s := string(body)
	for _, want := range []string{
		"proxies:",
		"name: amirreza-ws",
		"type: vless",
		"network: ws",
		"network: h2",
		"proxy-groups:",
		"name: ZeroOne",
		"rules:",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("Clash output missing %q\n---\n%s", want, s)
		}
	}
}

func TestNegotiateFormat(t *testing.T) {
	cases := []struct {
		name   string
		ua     string
		accept string
		query  string
		want   Format
	}{
		{"default → base64", "curl/7", "", "", FormatBase64},
		{"clash UA → clash", "ClashforAndroid", "", "", FormatClash},
		{"mihomo UA → clash", "mihomo", "", "", FormatClash},
		{"sing-box UA → sing-box", "sing-box 1.9", "", "", FormatSingBox},
		{"hiddify UA → sing-box", "Hiddify/2.0", "", "", FormatSingBox},
		{"accept json → sing-box", "curl/7", "application/json", "", FormatSingBox},
		{"accept yaml → clash", "curl/7", "application/x-yaml", "", FormatClash},
		{"?format=clash overrides UA", "sing-box", "", "clash", FormatClash},
		{"?format=base64 overrides UA", "ClashforAndroid", "", "base64", FormatBase64},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/?format="+c.query, nil)
			r.Header.Set("User-Agent", c.ua)
			r.Header.Set("Accept", c.accept)
			got := NegotiateFormat(r)
			if got != c.want {
				t.Fatalf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestUserInfoHeaderOmitsZeroes(t *testing.T) {
	cases := []struct {
		in   UserInfo
		want string
	}{
		{UserInfo{UploadBytes: 100, DownloadBytes: 200}, "upload=100; download=200"},
		{UserInfo{UploadBytes: 1, DownloadBytes: 2, TotalBytes: 100}, "upload=1; download=2; total=100"},
		{UserInfo{TotalBytes: 50, ExpireUnix: 1700000000}, "upload=0; download=0; total=50; expire=1700000000"},
	}
	for _, c := range cases {
		if got := c.in.Header(); got != c.want {
			t.Fatalf("got %q want %q", got, c.want)
		}
	}
}

func TestHandleSubscriptionTokenAuth(t *testing.T) {
	cfg := stack.Config{}
	cfg.Xray.Users = []stack.User{
		{Email: "alice", UUID: "00000000-0000-4000-8000-000000000000", Enabled: true, SubToken: "tok-alice", QuotaBytes: 1024},
	}
	cfg.Server.PublicIP = "1.2.3.4"
	cfg.Xray.Inbounds.VLESSWSPort = 443

	d := Deps{
		Config:        func() stack.Config { return cfg },
		LoadUsage:     func() (usage.UserState, error) { return usage.UserState{Totals: map[string]usage.Pair{"alice": {Uplink: 10, Downlink: 30}}}, nil },
		PortalBaseURL: func(*http.Request) string { return "https://test.example" },
	}

	t.Run("404 on bad token", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/sub/nope", nil)
		HandleSubscription(d)(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("got status %d", w.Code)
		}
	})

	t.Run("200 with subscription headers on good token", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/sub/tok-alice", nil)
		HandleSubscription(d)(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("got status %d body=%s", w.Code, w.Body)
		}
		ui := w.Header().Get("subscription-userinfo")
		if !strings.Contains(ui, "upload=10") || !strings.Contains(ui, "download=30") || !strings.Contains(ui, "total=1024") {
			t.Fatalf("subscription-userinfo header missing fields: %q", ui)
		}
		if w.Header().Get("profile-web-page-url") != "https://test.example/me/tok-alice" {
			t.Fatalf("profile-web-page-url: %q", w.Header().Get("profile-web-page-url"))
		}
		if !strings.HasPrefix(w.Header().Get("profile-title"), "base64:") {
			t.Fatalf("profile-title not base64-encoded: %q", w.Header().Get("profile-title"))
		}
	})
}

func TestHandlePortalServesHTMLWithUserData(t *testing.T) {
	cfg := stack.Config{}
	cfg.Xray.Users = []stack.User{
		{Email: "bob", UUID: "00000000-0000-4000-8000-000000000000", Enabled: true, SubToken: "tok-bob"},
	}
	cfg.Server.PublicIP = "1.2.3.4"
	cfg.Xray.Inbounds.VLESSWSPort = 443
	d := Deps{
		Config:        func() stack.Config { return cfg },
		LoadUsage:     func() (usage.UserState, error) { return usage.UserState{Totals: map[string]usage.Pair{}}, nil },
		PortalBaseURL: func(*http.Request) string { return "https://test.example" },
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/me/tok-bob", nil)
	HandlePortal(d)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("got status %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Hi bob") {
		t.Fatalf("portal HTML missing greeting; body head: %s", body[:200])
	}
	if !strings.Contains(body, "https://test.example/sub/tok-bob") {
		t.Fatalf("portal HTML missing sub URL")
	}
}
