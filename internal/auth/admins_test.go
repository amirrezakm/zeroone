package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !VerifyPassword("correct-horse-battery-staple", hash) {
		t.Errorf("VerifyPassword should match the original password")
	}
	if VerifyPassword("wrong-password", hash) {
		t.Errorf("VerifyPassword should reject a different password")
	}
	if VerifyPassword("", hash) {
		t.Errorf("VerifyPassword should reject empty password")
	}
}

func TestVerifyPasswordRejectsMalformed(t *testing.T) {
	for _, bad := range []string{
		"",
		"plaintext",
		"pbkdf2-sha256$abc",
		"unknown-alg$200000$abc$def",
		"pbkdf2-sha256$nope$YWJj$YWJj",
	} {
		if VerifyPassword("whatever", bad) {
			t.Errorf("VerifyPassword should reject malformed %q", bad)
		}
	}
}

func TestSessionRoundTrip(t *testing.T) {
	secret, err := NewSessionSecret()
	if err != nil {
		t.Fatalf("NewSessionSecret: %v", err)
	}
	tok, _, err := IssueSession(secret, "amir")
	if err != nil {
		t.Fatalf("IssueSession: %v", err)
	}
	if got := VerifySession(secret, tok); got != "amir" {
		t.Errorf("VerifySession = %q, want amir", got)
	}
	if got := VerifySession("different-secret-hex", tok); got != "" {
		t.Errorf("VerifySession with wrong secret should return empty, got %q", got)
	}
	// Tamper with the payload — signature must invalidate.
	tampered := "AAAA" + tok[4:]
	if got := VerifySession(secret, tampered); got != "" {
		t.Errorf("VerifySession of tampered token should return empty, got %q", got)
	}
}

func TestIssueSessionRejectsPipeInUsername(t *testing.T) {
	secret, _ := NewSessionSecret()
	if _, _, err := IssueSession(secret, "bad|name"); err == nil {
		t.Errorf("IssueSession should reject usernames containing '|'")
	}
}
