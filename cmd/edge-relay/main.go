// edge-relay is a tiny HTTPS-to-origin reverse proxy designed to run on a
// managed PaaS (runflare, liara, hamravesh) that only exposes HTTP(S) on a
// platform-managed domain. The PaaS terminates TLS in front; this binary
// receives plain HTTP, forwards WebSocket and XHTTP requests to the Xray
// origin server, and serves a benign landing page on everything else.
//
// All wiring is env-driven so the same binary can front any number of edge
// providers without code changes.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type route struct {
	Path   string
	Origin *url.URL
	Host   string // optional Host header override
	Label  string // for logs
}

type config struct {
	Listen      string
	Routes      []route
	LandingBody string
	LogLevel    string
}

func loadConfig() (config, error) {
	listen := os.Getenv("LISTEN_ADDR")
	if listen == "" {
		port := envOr("PORT", "8080")
		if _, err := strconv.Atoi(port); err != nil {
			return config{}, fmt.Errorf("PORT not numeric: %q", port)
		}
		listen = ":" + port
	}

	originHost := os.Getenv("ORIGIN_HOST")

	var routes []route
	add := func(label, path, target, hostOverride string) error {
		path = strings.TrimSpace(path)
		target = strings.TrimSpace(target)
		if path == "" || target == "" {
			return nil
		}
		if !strings.HasPrefix(path, "/") {
			return fmt.Errorf("%s path %q must start with /", label, path)
		}
		u, err := url.Parse(target)
		if err != nil {
			return fmt.Errorf("%s target %q: %w", label, target, err)
		}
		if u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("%s target %q must be a full URL (scheme://host[:port])", label, target)
		}
		routes = append(routes, route{Path: path, Origin: u, Host: hostOverride, Label: label})
		return nil
	}

	wsPath := envOr("WS_PATH", "/vless")
	wsTarget := os.Getenv("WS_TARGET")
	if wsTarget == "" && originHost != "" {
		wsTarget = "http://" + joinHostPort(originHost, "443")
	}
	if err := add("ws", wsPath, wsTarget, os.Getenv("WS_HOST")); err != nil {
		return config{}, err
	}

	xhttpPath := envOr("XHTTP_PATH", "/api/v1/events")
	xhttpTarget := os.Getenv("XHTTP_TARGET")
	if xhttpTarget == "" && originHost != "" {
		xhttpTarget = "http://" + joinHostPort(originHost, "80")
	}
	if err := add("xhttp", xhttpPath, xhttpTarget, os.Getenv("XHTTP_HOST")); err != nil {
		return config{}, err
	}

	// /sub/ and /me/ go through the panel's subscription + portal handlers
	// at the same nginx :80 used by xhttp, then nginx proxies to zeroone
	// on 127.0.0.1:8091.
	panelTarget := os.Getenv("PANEL_TARGET")
	if panelTarget == "" && originHost != "" {
		panelTarget = "http://" + joinHostPort(originHost, "80")
	}
	if err := add("sub", envOr("SUB_PATH", "/sub/"), panelTarget, os.Getenv("PANEL_HOST")); err != nil {
		return config{}, err
	}
	if err := add("portal", envOr("PORTAL_PATH", "/me/"), panelTarget, os.Getenv("PANEL_HOST")); err != nil {
		return config{}, err
	}

	// EDGE_EXTRA_ROUTES allows additional path→target mappings without
	// editing the binary. Format: "label:/path=scheme://host[:port][,Host=override][;...]"
	// Example: "ws2:/alt-ws=http://1.2.3.4:443,Host=origin.example.com"
	if extra := os.Getenv("EDGE_EXTRA_ROUTES"); extra != "" {
		for _, spec := range strings.Split(extra, ";") {
			spec = strings.TrimSpace(spec)
			if spec == "" {
				continue
			}
			label, rest, ok := strings.Cut(spec, ":")
			if !ok {
				return config{}, fmt.Errorf("EDGE_EXTRA_ROUTES entry %q missing label", spec)
			}
			pathPart, targetPart, ok := strings.Cut(rest, "=")
			if !ok {
				return config{}, fmt.Errorf("EDGE_EXTRA_ROUTES entry %q missing =", spec)
			}
			var target, hostOverride string
			if t, h, ok := strings.Cut(targetPart, ",Host="); ok {
				target, hostOverride = t, h
			} else {
				target = targetPart
			}
			if err := add(strings.TrimSpace(label), pathPart, target, hostOverride); err != nil {
				return config{}, err
			}
		}
	}

	if len(routes) == 0 {
		return config{}, errors.New("no routes configured; set ORIGIN_HOST or WS_TARGET/XHTTP_TARGET")
	}

	// More specific paths first so http.ServeMux's longest-prefix match
	// behaves predictably when paths overlap.
	sort.SliceStable(routes, func(i, j int) bool {
		return len(routes[i].Path) > len(routes[j].Path)
	})

	return config{
		Listen:      listen,
		Routes:      routes,
		LandingBody: envOr("LANDING_BODY", "OK"),
		LogLevel:    envOr("LOG_LEVEL", "info"),
	}, nil
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "edge-relay:", err)
		os.Exit(2)
	}
	configureLogger(cfg.LogLevel)

	mux := http.NewServeMux()
	for _, r := range cfg.Routes {
		proxy := newProxy(r)
		// Match both the exact path and any subpath. xhttp clients address
		// per-session subpaths under the configured base (e.g.
		// /api/v1/events/<session-id>); WS clients always hit the exact
		// path. http.ServeMux treats a trailing-slash pattern as a subtree
		// and a non-slash pattern as exact, so register both — they don't
		// conflict and avoid the auto-redirect a bare subtree match would
		// emit for the exact path.
		mux.Handle(r.Path, proxy)
		if !strings.HasSuffix(r.Path, "/") {
			mux.Handle(r.Path+"/", proxy)
		}
		slog.Info("route", "label", r.Label, "path", r.Path, "origin", r.Origin.String())
	}
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Anything not matched above (including bare "/") returns the
		// landing body with cache disabled and a noindex header so scanners
		// don't fingerprint this domain as a proxy.
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(cfg.LandingBody))
	})

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
		// xhttp stream-up keeps a single request open for the lifetime of
		// a session. Don't impose ReadTimeout / WriteTimeout — let the
		// origin and client decide. IdleTimeout still trims dead keep-alives.
		IdleTimeout: 120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.Listen, "routes", len(cfg.Routes))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutting down")
	case err := <-errCh:
		slog.Error("listen", "err", err)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

func newProxy(r route) http.Handler {
	rp := httputil.NewSingleHostReverseProxy(r.Origin)
	// FlushInterval is a latency/throughput tradeoff:
	//   -1 → flush after every Write (lowest ping, worst throughput; many syscalls)
	//   >0 → flush every interval (batched writes, fewer syscalls)
	//   0 → Go default (lets net/http's bufio decide)
	// xhttp tunnel traffic is mostly bulk data transfer where throughput
	// dominates user experience; we batch with a small interval so chunks
	// still ship promptly but small per-message overhead is amortized.
	rp.FlushInterval = 50 * time.Millisecond
	director := rp.Director
	rp.Director = func(req *http.Request) {
		director(req)
		// Preserve the original request path verbatim. NewSingleHostReverseProxy's
		// default director joins the origin path with the request path; for
		// our use case (matched-prefix passthrough), we want the request URL
		// as-is.
		if r.Host != "" {
			req.Host = r.Host
		}
		// Strip provider-injected client metadata we don't want forwarded.
		// runflare passes through CF-style headers; xray's sniffing logic
		// won't use them, but leaking them is unnecessary.
		req.Header.Del("CF-Connecting-IP")
		req.Header.Del("CF-Ray")
		req.Header.Del("CF-IPCountry")
	}
	rp.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
		// http.ErrAbortHandler propagates the original abort path; don't
		// overwrite the response in that case.
		if errors.Is(err, http.ErrAbortHandler) {
			return
		}
		slog.Warn("proxy error", "label", r.Label, "path", req.URL.Path, "err", err.Error())
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}
	rp.ModifyResponse = func(resp *http.Response) error {
		// Tell any nginx in front of us (runflare, liara, etc.) to flush
		// the response immediately rather than buffer. xhttp stream-up mode
		// keeps a single response open for tens of seconds and dribbles
		// chunks down — default nginx `proxy_buffering on` collects those
		// chunks until the response is "done", which the client's xray
		// reads as "no data" → `context deadline exceeded`.
		resp.Header.Set("X-Accel-Buffering", "no")
		return nil
	}
	// Custom Transport tuned for xhttp:
	//   1. xray closes the long-lived stream after `sc_stream_up_server_secs`
	//      (20–80s). The TCP conn that carried the stream then dies. We can't
	//      let pooled conns live longer than that — IdleConnTimeout=15s evicts
	//      them well before xray's 20s minimum, so reuse is safe.
	//   2. ResponseHeaderTimeout stays 0: stream-up GETs may take many seconds
	//      before the first chunk arrives.
	//   3. Keep-alives ON so back-to-back POSTs in an xhttp upload burst can
	//      reuse the same TCP and skip the handshake (~1–2 ms saved per req).
	//      Previously this was disabled to dodge stale-conn errors, but a
	//      tight IdleConnTimeout solves that without paying the handshake on
	//      every request.
	rp.Transport = &http.Transport{
		MaxIdleConns:          512,
		MaxIdleConnsPerHost:   128,
		MaxConnsPerHost:       0, // unlimited concurrent dials
		IdleConnTimeout:       15 * time.Second,
		ResponseHeaderTimeout: 0,
		ExpectContinueTimeout: time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		WriteBufferSize:       64 * 1024,
		ReadBufferSize:        64 * 1024,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	// ReverseProxy already supports WebSocket upgrades transparently
	// (since Go 1.12), so no extra wiring is needed for the /vless path.
	return rp
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func joinHostPort(host, port string) string {
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		return host
	}
	return host + ":" + port
}

func configureLogger(level string) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})))
}
