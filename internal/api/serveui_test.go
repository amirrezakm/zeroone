package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/amirrezakm/zeroone/internal/stack"
)

// TestServeUIRejectsPathTraversal locks in the path-injection fix for the SPA
// file server: no request path may escape the configured UI root, however it is
// spelled (raw "..", nested "..", or percent-encoded forms). A secret file is
// planted next to — but outside — the UI root; serveUI must never return it.
func TestServeUIRejectsPathTraversal(t *testing.T) {
	base := t.TempDir()
	uiDir := filepath.Join(base, "ui")
	if err := os.MkdirAll(filepath.Join(uiDir, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir ui: %v", err)
	}
	write := func(p, body string) {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	write(filepath.Join(uiDir, "index.html"), "INDEX")
	write(filepath.Join(uiDir, "assets", "app.js"), "ASSET")
	// Planted outside the UI root: serving this means traversal succeeded.
	write(filepath.Join(base, "secret.txt"), "SECRET")

	var cfg stack.Config
	cfg.Server.UIPath = uiDir
	s := &Server{cfg: cfg}

	// rawPathReq sets URL.Path directly so the handler sees the exact path
	// (bypassing ServeMux's own cleaning) — this is what serveUI must defend.
	rawPathReq := func(p string) *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		r.URL.Path = p
		return r
	}

	t.Run("legitimate paths are served", func(t *testing.T) {
		cases := map[string]string{
			"/":              "INDEX",
			"/index.html":    "INDEX",
			"/assets/app.js": "ASSET",
		}
		for path, want := range cases {
			w := httptest.NewRecorder()
			s.serveUI(w, rawPathReq(path))
			if got := w.Body.String(); got != want {
				t.Errorf("serveUI(%q) = %q, want %q (status %d)", path, got, want, w.Code)
			}
		}
	})

	t.Run("traversal never escapes the UI root", func(t *testing.T) {
		// Mix of raw and percent-encoded traversal attempts. None may return
		// the planted secret; each must 404 or fall back to the app shell.
		attempts := []*http.Request{
			rawPathReq("/../secret.txt"),
			rawPathReq("/../../secret.txt"),
			rawPathReq("/assets/../../secret.txt"),
			rawPathReq("/..%2f..%2fsecret.txt"), // URL.Path keeps this literal
			httptest.NewRequest("GET", "/%2e%2e/secret.txt", nil),
			httptest.NewRequest("GET", "/..%2f..%2fsecret.txt", nil),
		}
		for _, r := range attempts {
			w := httptest.NewRecorder()
			s.serveUI(w, r)
			if body := w.Body.String(); body == "SECRET" {
				t.Errorf("serveUI(%q) leaked file outside UI root", r.URL.Path)
			}
			if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
				t.Errorf("serveUI(%q) status = %d, want 200 (shell) or 404", r.URL.Path, w.Code)
			}
		}
	})
}
