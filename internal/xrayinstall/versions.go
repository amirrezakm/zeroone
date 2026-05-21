// SPDX-License-Identifier: AGPL-3.0-or-later
package xrayinstall

import (
	"regexp"
	"strconv"
	"strings"
)

// versionTokenRE is the single source of truth for what we will accept
// as a filesystem-safe version token. The pattern intentionally
// excludes path separators, `..`, and any non-printable bytes — and
// is recognised by CodeQL's go/path-injection query as a sanitiser
// when used via regexp.Regexp.MatchString at the path-construction
// site (not just inside a helper).
var versionTokenRE = regexp.MustCompile(`^[A-Za-z0-9._+\-]{1,64}$`)

// ValidateVersionToken returns an error when v is unsafe to use as a
// filesystem path component. The token is what gets joined into
// versions/<v>/xray, so anything containing path separators, `..`, or
// non-printable characters has to be rejected — otherwise a malicious
// mirror returning a crafted release tag, or an admin upload labelled
// `../../etc`, could write outside the override tree.
func ValidateVersionToken(v string) error {
	if !versionTokenRE.MatchString(v) {
		return errInvalidVersion
	}
	if v == "." || v == ".." || strings.Contains(v, "..") {
		return errInvalidVersion
	}
	return nil
}

var errInvalidVersion = errInvalidVersionT{}

type errInvalidVersionT struct{}

func (errInvalidVersionT) Error() string {
	return "xrayinstall: invalid version token (allowed: alnum, '.', '-', '_', '+', max 64 chars, no '..' or path separators)"
}

// CompareVersions returns -1/0/1 like strings.Compare, on the SemVer
// MAJOR.MINOR.PATCH portion. Unknown / unparseable inputs sort last
// (returned as 0 vs known so the panel never claims a known version is
// older than an unknown one).
func CompareVersions(a, b string) int {
	pa, oka := parseSemver(a)
	pb, okb := parseSemver(b)
	if !oka && !okb {
		return 0
	}
	if !oka {
		return -1
	}
	if !okb {
		return 1
	}
	for k := 0; k < 3; k++ {
		if pa[k] < pb[k] {
			return -1
		}
		if pa[k] > pb[k] {
			return 1
		}
	}
	return 0
}

func parseSemver(s string) ([3]int, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return [3]int{}, false
	}
	var out [3]int
	for k := 0; k < 3; k++ {
		if k >= len(parts) {
			out[k] = 0
			continue
		}
		n, err := strconv.Atoi(parts[k])
		if err != nil {
			return [3]int{}, false
		}
		out[k] = n
	}
	return out, true
}
