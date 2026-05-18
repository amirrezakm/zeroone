// Cookie-based session tokens for the panel login flow.
//
// Token format: base64url(payload) + "." + base64url(HMAC-SHA256(payload, secret))
// where payload is base64url(<username>|<expires_unix>). Username is restricted
// to printable ASCII without "|" so the split is unambiguous.
//
// The cookie is httpOnly + SameSite=Lax. It is marked Secure when the request
// came in over TLS (direct or via X-Forwarded-Proto), so plain-HTTP setups
// (local dev, IP-only servers) keep working without manual flag tweaking.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	SessionCookieName = "xray_stack_session"
	SessionTTL        = 12 * time.Hour
)

// NewSessionSecret returns a 32-byte hex string suitable for signing
// session cookies. Persisted in the panel config so tokens survive
// restarts.
func NewSessionSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// IssueSession builds a signed session token for the given username,
// valid for SessionTTL from now.
func IssueSession(secret, username string) (string, time.Time, error) {
	if secret == "" {
		return "", time.Time{}, fmt.Errorf("session secret not configured")
	}
	if strings.ContainsAny(username, "|") {
		return "", time.Time{}, fmt.Errorf("username contains reserved character")
	}
	expires := time.Now().Add(SessionTTL)
	payload := fmt.Sprintf("%s|%d", username, expires.Unix())
	enc := base64.RawURLEncoding.EncodeToString([]byte(payload))
	sig := hmacSig(secret, enc)
	return enc + "." + sig, expires, nil
}

// VerifySession validates a token and returns the username on success.
// Returns "" if the token is missing, malformed, expired, or signature
// doesn't match.
func VerifySession(secret, token string) string {
	if secret == "" || token == "" {
		return ""
	}
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return ""
	}
	want := hmacSig(secret, parts[0])
	if !hmac.Equal([]byte(want), []byte(parts[1])) {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return ""
	}
	idx := strings.LastIndexByte(string(raw), '|')
	if idx < 0 {
		return ""
	}
	username := string(raw[:idx])
	expStr := string(raw[idx+1:])
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return ""
	}
	if time.Now().Unix() > exp {
		return ""
	}
	return username
}

func hmacSig(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// SetSessionCookie writes the session cookie to the response. Marks
// the cookie Secure when the request arrived over TLS.
func SetSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsTLS(r),
	})
}

// ClearSessionCookie expires the session cookie immediately.
func ClearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsTLS(r),
	})
}

// SessionFromRequest extracts the session cookie's verified username, or
// "" if no valid cookie is present.
func SessionFromRequest(r *http.Request, secret string) string {
	c, err := r.Cookie(SessionCookieName)
	if err != nil || c == nil {
		return ""
	}
	return VerifySession(secret, c.Value)
}

func requestIsTLS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if p := r.Header.Get("X-Forwarded-Proto"); strings.EqualFold(p, "https") {
		return true
	}
	return false
}
