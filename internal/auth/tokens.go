// Package auth implements API token authentication for the panel.
// Tokens are stored hashed (SHA-256) in stack.json. The plaintext is shown
// to the operator only at creation time. The middleware accepts either:
//   - An Authorization: Bearer <token> header that matches a stored hash
//   - No Authorization header at all (falls through to nginx Basic Auth)
//
// A request with a Bearer header that doesn't match is rejected with 401
// so a leaked-but-revoked token cannot silently fall back to Basic Auth.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

// Hash returns the SHA-256 hex digest used for storage and comparison.
func Hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Generate returns a fresh 32-byte hex token.
func Generate() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// LookupHash returns the stored hash matching the bearer header, or "" if
// no Bearer header was provided (in which case the request is allowed —
// nginx Basic Auth, or any other front-line check, is responsible).
//
// Only a Bearer header that is *present but invalid* causes a rejection.
// A Basic Auth header (or any other Authorization scheme) is left
// untouched so the existing nginx htpasswd flow keeps working.
func LookupHash(r *http.Request, knownHashes []string) (string, bool) {
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		return "", true
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" {
		return "", false
	}
	candidate := Hash(token)
	for _, h := range knownHashes {
		if h == candidate {
			return h, true
		}
	}
	return "", false
}
