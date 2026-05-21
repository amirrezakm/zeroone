// SPDX-License-Identifier: AGPL-3.0-or-later
package xrayinstall

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// FileSHA256 returns the lower-case hex sha256 of the file at path.
// Used both for verifying a freshly-downloaded zip and for stamping
// the installed binary's checksum into state.json.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ParseDigest extracts the sha256 hex from an Xray-core ".dgst" file.
// The file is multi-line, with at least one line of the form
//
//	SHA2-256= <hex>
//	SHA2-512= <hex>
//
// We accept either "SHA2-256" or "SHA256" prefixes, case-insensitive,
// to be forgiving across upstream format changes. Returns the first
// matching sha256 hex (lower-case) or an error if none is present.
func ParseDigest(body []byte) (string, error) {
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		upper := strings.ToUpper(line)
		if !strings.HasPrefix(upper, "SHA2-256") && !strings.HasPrefix(upper, "SHA256") {
			continue
		}
		// Split on either `=` or whitespace following the algorithm tag.
		rest := line
		for _, sep := range []string{"=", ":"} {
			if i := strings.Index(rest, sep); i >= 0 {
				rest = rest[i+1:]
				break
			}
		}
		rest = strings.TrimSpace(rest)
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		candidate := strings.ToLower(fields[len(fields)-1])
		if isHex64(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("xrayinstall: no SHA2-256 digest found in .dgst body")
}

func isHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}
