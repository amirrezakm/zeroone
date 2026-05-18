// Admin password hashing. Uses PBKDF2-HMAC-SHA256 (Go 1.24+ stdlib) with
// a per-password random 16-byte salt and 200_000 iterations. Hash format:
//
//	pbkdf2-sha256$<iterations>$<base64(salt)>$<base64(hash)>
//
// The format prefix lets us migrate to a stronger KDF later without
// invalidating existing hashes — Verify rejects unknown prefixes.
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

const (
	pbkdf2Prefix     = "pbkdf2-sha256"
	pbkdf2Iterations = 200_000
	pbkdf2KeyLen     = 32
	pbkdf2SaltLen    = 16
)

// HashPassword returns an encoded PBKDF2-SHA256 hash of the given password.
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", fmt.Errorf("password is required")
	}
	salt := make([]byte, pbkdf2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, pbkdf2Iterations, pbkdf2KeyLen)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s$%d$%s$%s",
		pbkdf2Prefix,
		pbkdf2Iterations,
		base64.StdEncoding.EncodeToString(salt),
		base64.StdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword compares a plaintext password against a stored hash.
// Returns true on match, false on mismatch or malformed hash.
func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != pbkdf2Prefix {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 {
		return false
	}
	salt, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}
