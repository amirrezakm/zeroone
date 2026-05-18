package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

// startOrigin spins up a fake Xray origin that handles both an HTTP echo at
// /api/v1/events (xhttp) and a WS echo at /vless.
func startOrigin(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	xhttpHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Origin-Got-Path", r.URL.Path)
		w.Header().Set("X-Origin-Got-Host", r.Host)
		body, _ := io.ReadAll(r.Body)
		_, _ = w.Write(body)
	}
	mux.HandleFunc("/api/v1/events", xhttpHandler)
	mux.HandleFunc("/api/v1/events/", xhttpHandler)
	mux.Handle("/vless", websocket.Handler(func(ws *websocket.Conn) {
		buf := make([]byte, 64*1024)
		for {
			_ = ws.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, err := ws.Read(buf)
			if err != nil {
				return
			}
			if _, err := ws.Write(buf[:n]); err != nil {
				return
			}
		}
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// startRelay builds an *http.Server from loadConfig with the given env and
// serves it on a local listener. Returns the relay base URL.
func startRelay(t *testing.T, env map[string]string) string {
	t.Helper()
	for k, v := range env {
		t.Setenv(k, v)
	}
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	mux := http.NewServeMux()
	for _, r := range cfg.Routes {
		proxy := newProxy(r)
		mux.Handle(r.Path, proxy)
		if !strings.HasSuffix(r.Path, "/") {
			mux.Handle(r.Path+"/", proxy)
		}
	}
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(cfg.LandingBody)) })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return "http://" + ln.Addr().String()
}

func TestXHTTPRouteForwardsPathAndBody(t *testing.T) {
	origin := startOrigin(t)
	relay := startRelay(t, map[string]string{
		"LISTEN_ADDR":  "",
		"WS_PATH":      "",
		"WS_TARGET":    "",
		"XHTTP_PATH":   "/api/v1/events",
		"XHTTP_TARGET": origin.URL,
	})
	body := strings.NewReader("hello-xhttp")
	req, _ := http.NewRequest("POST", relay+"/api/v1/events", body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if string(got) != "hello-xhttp" {
		t.Fatalf("body roundtrip mismatch: got %q", got)
	}
	if h := resp.Header.Get("X-Origin-Got-Path"); h != "/api/v1/events" {
		t.Fatalf("origin saw path %q, want /api/v1/events", h)
	}
}

func TestXHTTPRouteForwardsSubPaths(t *testing.T) {
	// xhttp clients address per-session subpaths under the configured path
	// (e.g. /api/v1/events/<session-id>). The proxy must forward those, not
	// fall through to the catchall landing handler.
	origin := startOrigin(t)
	relay := startRelay(t, map[string]string{
		"WS_PATH":      "",
		"WS_TARGET":    "",
		"XHTTP_PATH":   "/api/v1/events",
		"XHTTP_TARGET": origin.URL,
	})
	for _, sub := range []string{"/abc123", "/sess-xyz", "/a/b/c"} {
		full := relay + "/api/v1/events" + sub
		req, _ := http.NewRequest("POST", full, strings.NewReader("body"))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("post %s: %v", full, err)
		}
		got, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if string(got) != "body" {
			t.Fatalf("subpath %s: body roundtrip mismatch %q", sub, got)
		}
		if h := resp.Header.Get("X-Origin-Got-Path"); h != "/api/v1/events"+sub {
			t.Fatalf("subpath %s: origin saw %q, want /api/v1/events%s", sub, h, sub)
		}
	}
}

func TestWSRouteUpgradesAndEchoes(t *testing.T) {
	origin := startOrigin(t)
	relay := startRelay(t, map[string]string{
		"WS_PATH":      "/vless",
		"WS_TARGET":    origin.URL,
		"XHTTP_PATH":   "",
		"XHTTP_TARGET": "",
	})

	u, _ := url.Parse(relay + "/vless")
	wsURL := "ws://" + u.Host + u.Path
	cfg, err := websocket.NewConfig(wsURL, "http://localhost/")
	if err != nil {
		t.Fatalf("ws config: %v", err)
	}
	conn, err := websocket.DialConfig(cfg)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("ws write: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 32)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("ws read: %v", err)
	}
	if got := string(buf[:n]); got != "ping" {
		t.Fatalf("ws echo: got %q want %q", got, "ping")
	}
}

func TestHostOverrideIsForwarded(t *testing.T) {
	origin := startOrigin(t)
	relay := startRelay(t, map[string]string{
		"XHTTP_PATH":   "/api/v1/events",
		"XHTTP_TARGET": origin.URL,
		"XHTTP_HOST":   "edge.example.com",
		"WS_PATH":      "",
		"WS_TARGET":    "",
	})
	resp, err := http.Post(relay+"/api/v1/events", "text/plain", strings.NewReader(""))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if h := resp.Header.Get("X-Origin-Got-Host"); h != "edge.example.com" {
		t.Fatalf("origin Host header was %q, want edge.example.com", h)
	}
}

func TestUnmatchedPathReturnsLanding(t *testing.T) {
	origin := startOrigin(t)
	relay := startRelay(t, map[string]string{
		"XHTTP_PATH":   "/api/v1/events",
		"XHTTP_TARGET": origin.URL,
		"WS_PATH":      "",
		"WS_TARGET":    "",
		"LANDING_BODY": "edge-online",
	})
	resp, err := http.Get(relay + "/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if string(got) != "edge-online" {
		t.Fatalf("landing body: got %q want edge-online", got)
	}
}

func TestLoadConfigRequiresAtLeastOneRoute(t *testing.T) {
	t.Setenv("WS_PATH", "")
	t.Setenv("WS_TARGET", "")
	t.Setenv("XHTTP_PATH", "")
	t.Setenv("XHTTP_TARGET", "")
	t.Setenv("ORIGIN_HOST", "")
	t.Setenv("EDGE_EXTRA_ROUTES", "")
	t.Setenv("PORT", "8080")
	if _, err := loadConfig(); err == nil {
		t.Fatalf("expected error when no routes configured")
	}
}

func TestExtraRoutesAreParsed(t *testing.T) {
	t.Setenv("WS_PATH", "")
	t.Setenv("WS_TARGET", "")
	t.Setenv("XHTTP_PATH", "")
	t.Setenv("XHTTP_TARGET", "")
	t.Setenv("ORIGIN_HOST", "")
	t.Setenv("EDGE_EXTRA_ROUTES", "alt:/foo=http://1.2.3.4:80;ws2:/bar=http://5.6.7.8:443,Host=upstream.test")
	t.Setenv("PORT", "8080")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if len(cfg.Routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(cfg.Routes))
	}
	for _, r := range cfg.Routes {
		switch r.Label {
		case "alt":
			if r.Path != "/foo" || r.Origin.Host != "1.2.3.4:80" {
				t.Fatalf("alt route wrong: %+v", r)
			}
		case "ws2":
			if r.Path != "/bar" || r.Origin.Host != "5.6.7.8:443" || r.Host != "upstream.test" {
				t.Fatalf("ws2 route wrong: %+v", r)
			}
		default:
			t.Fatalf("unexpected label %q", r.Label)
		}
	}
}
