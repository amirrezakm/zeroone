package subscription

import (
	"mime"
	"strings"
	"testing"
)

// TestSafeAttachmentName locks in the sanitizer that feeds the /sub
// Content-Disposition filename: arbitrary user emails must never carry CR/LF,
// quotes, semicolons, or path separators into the header, and empty/unicode
// inputs must degrade to a usable token.
func TestSafeAttachmentName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain email", "alice@example.com", "alice_example.com"},
		{"crlf injection", "a\r\nb@x.com", "a__b_x.com"},
		{"quotes", `a"b'@x.com`, "a_b__x.com"},
		{"semicolon params", "a;filename=evil@x.com", "a_filename_evil_x.com"},
		{"slashes", "../../etc/passwd", "etc_passwd"},
		{"backslash", `a\b@x.com`, "a_b_x.com"},
		{"unicode only", "تست‌کاربر", "subscription"},
		{"empty", "", "subscription"},
		{"only specials", "@@@", "subscription"},
		{"leading trailing punct", "._-bob-_.", "bob"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := safeAttachmentName(tc.in)
			if got != tc.want {
				t.Fatalf("safeAttachmentName(%q) = %q, want %q", tc.in, got, tc.want)
			}
			// Whatever the input, the result must contain no character that
			// could break out of the header value.
			if strings.ContainsAny(got, "\r\n\";/\\ ") {
				t.Fatalf("safeAttachmentName(%q) = %q leaked an unsafe char", tc.in, got)
			}
		})
	}
}

// TestSafeAttachmentNameCaps verifies the 80-byte cap so a pathological email
// can't bloat the header.
func TestSafeAttachmentNameCaps(t *testing.T) {
	got := safeAttachmentName(strings.Repeat("a", 500))
	if len(got) != 80 {
		t.Fatalf("len = %d, want 80", len(got))
	}
}

// TestContentDispositionRoundTrip proves the full header value built from a
// hostile email parses back to a single, clean filename with no smuggled
// parameters — the property the /sub handler relies on.
func TestContentDispositionRoundTrip(t *testing.T) {
	hostile := "victim\r\nSet-Cookie: x=1\";filename=\"evil@host.com"
	filename := safeAttachmentName(hostile) + "-base64.txt"
	header := mime.FormatMediaType("attachment", map[string]string{"filename": filename})
	if header == "" {
		t.Fatal("FormatMediaType returned empty header")
	}
	if strings.ContainsAny(header, "\r\n") {
		t.Fatalf("header contains a line break: %q", header)
	}
	mediatype, params, err := mime.ParseMediaType(header)
	if err != nil {
		t.Fatalf("ParseMediaType(%q): %v", header, err)
	}
	if mediatype != "attachment" {
		t.Fatalf("mediatype = %q, want attachment", mediatype)
	}
	if len(params) != 1 || params["filename"] != filename {
		t.Fatalf("params = %v, want single filename=%q", params, filename)
	}
}
