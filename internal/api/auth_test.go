package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amirrezakm/zeroone/internal/auth"
	"github.com/amirrezakm/zeroone/internal/stack"
)

// TestAuthGateReflectsAdminAddOnDisk locks in the regression for the
// install-time security bug: the install script writes the admin
// straight to stack.json via the CLI, so the running daemon must pick
// it up without a restart. Before the fix, the daemon kept serving
// every endpoint unauthenticated until the operator manually restarted.
func TestAuthGateReflectsAdminAddOnDisk(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "stack.json")
	writeBootstrapConfig(t, configPath)

	cfg, err := stack.Load(configPath)
	if err != nil {
		t.Fatalf("load bootstrap config: %v", err)
	}
	handler := NewServer(*cfg, configPath, true)

	// 1. Bootstrap window from a non-loopback caller: gate must close.
	resp := do(handler, newReq(t, "GET", "/api/admins", "203.0.113.7:55555", "", nil))
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("remote bootstrap should require login, got %d", resp.Code)
	}

	// 2. Bootstrap window from loopback: open path is still allowed so
	//    the installer can seed the first admin via curl on localhost.
	resp = do(handler, newReq(t, "GET", "/api/admins", "127.0.0.1:55556", "", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("loopback bootstrap should be allowed, got %d body=%s", resp.Code, resp.Body.String())
	}

	// 3. Simulate `zeroone admin add` writing directly to stack.json
	//    while the daemon is still running.
	addAdminToFile(t, configPath, "alice", "correct-horse-staple")

	// 4. /api/me must immediately see the new admin from disk.
	resp = do(handler, newReq(t, "GET", "/api/me", "127.0.0.1:55557", "", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("/api/me after admin add: %d body=%s", resp.Code, resp.Body.String())
	}
	var me map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &me); err != nil {
		t.Fatalf("decode /api/me: %v", err)
	}
	if got, _ := me["bootstrap_needed"].(bool); got {
		t.Fatalf("/api/me should report bootstrap_needed=false after admin add, got %v", me)
	}
	if got, _ := me["admins_count"].(float64); got != 1 {
		t.Fatalf("/api/me admins_count = %v, want 1", got)
	}

	// 5. Any mutating endpoint hit without credentials must now be
	//    rejected, even from loopback — the open bootstrap path closes
	//    the instant an admin exists.
	resp = do(handler, newReq(t, "GET", "/api/admins", "127.0.0.1:55558", "", nil))
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("after admin add, anonymous calls must require login; got %d body=%s", resp.Code, resp.Body.String())
	}

	// 6. Logging in with the freshly-added credentials must succeed and
	//    issue a session cookie that unlocks the same endpoint.
	loginBody := strings.NewReader(`{"username":"alice","password":"correct-horse-staple"}`)
	resp = do(handler, newReq(t, "POST", "/api/login", "127.0.0.1:55559", "", loginBody))
	if resp.Code != http.StatusOK {
		t.Fatalf("login after admin add should succeed, got %d body=%s", resp.Code, resp.Body.String())
	}
	cookie := pickSessionCookie(resp.Result().Cookies())
	if cookie == "" {
		t.Fatalf("login response had no session cookie")
	}

	resp = do(handler, newReq(t, "GET", "/api/admins", "127.0.0.1:55560", cookie, nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("authenticated /api/admins should succeed, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestIsLoopbackRemote(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:9000", true},
		{"[::1]:9000", true},
		{"localhost:9000", true},
		{"10.0.0.5:9000", false},
		{"203.0.113.1:80", false},
		{"", false},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/api/me", nil)
		r.RemoteAddr = c.addr
		if got := isLoopbackRemote(r); got != c.want {
			t.Errorf("isLoopbackRemote(%q) = %v, want %v", c.addr, got, c.want)
		}
	}
}

func writeBootstrapConfig(t *testing.T, path string) {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", "cmd", "zeroone", "minimal-stack.json"))
	if err != nil {
		t.Fatalf("read minimal-stack.json: %v", err)
	}
	if err := os.WriteFile(path, src, 0o600); err != nil {
		t.Fatalf("write bootstrap config: %v", err)
	}
}

func addAdminToFile(t *testing.T, path, username, password string) {
	t.Helper()
	cfg, err := stack.Load(path)
	if err != nil {
		t.Fatalf("load for admin add: %v", err)
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	cfg.Panel.Admins = append(cfg.Panel.Admins, stack.Admin{
		Username:     username,
		PasswordHash: hash,
		CreatedAt:    1,
	})
	if err := stack.Save(path, *cfg); err != nil {
		t.Fatalf("save after admin add: %v", err)
	}
}

func newReq(t *testing.T, method, path, remote, sessionCookie string, body *strings.Reader) *http.Request {
	t.Helper()
	var r *http.Request
	if body == nil {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, body)
		r.Header.Set("Content-Type", "application/json")
	}
	r.RemoteAddr = remote
	if sessionCookie != "" {
		r.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: sessionCookie})
	}
	return r
}

func do(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func pickSessionCookie(cookies []*http.Cookie) string {
	for _, c := range cookies {
		if c.Name == auth.SessionCookieName {
			return c.Value
		}
	}
	return ""
}
