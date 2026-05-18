package subscription

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
)

// UserInfo carries the per-user counters and limits that get serialized
// into the `subscription-userinfo` response header. Clients (Hiddify,
// Clash Meta, v2rayN, NekoBox) read this header to show a usage bar.
//
// Bytes are absolute counters; Total=0 means unlimited; Expire=0 means
// no expiry.
type UserInfo struct {
	UploadBytes   int64
	DownloadBytes int64
	TotalBytes    int64 // quota; 0 = unlimited
	ExpireUnix    int64 // 0 = no expiry
}

// Header formats UserInfo as the standard `upload=...; download=...; ...`
// value. Always includes upload+download; omits total/expire when zero so
// clients show "unlimited" rather than "0 bytes used of 0".
func (u UserInfo) Header() string {
	parts := []string{
		fmt.Sprintf("upload=%d", u.UploadBytes),
		fmt.Sprintf("download=%d", u.DownloadBytes),
	}
	if u.TotalBytes > 0 {
		parts = append(parts, fmt.Sprintf("total=%d", u.TotalBytes))
	}
	if u.ExpireUnix > 0 {
		parts = append(parts, fmt.Sprintf("expire=%d", u.ExpireUnix))
	}
	return strings.Join(parts, "; ")
}

// WriteResponseHeaders sets the headers every well-behaved subscription
// response should carry. webPageURL points clients at the user portal
// (rendered with a "manage" button by Hiddify et al.); title is the
// display name; updateIntervalHours suggests how often the client should
// poll for refreshed config (24 is a sane default).
func WriteResponseHeaders(h http.Header, info UserInfo, title, webPageURL string, updateIntervalHours int) {
	h.Set("subscription-userinfo", info.Header())
	if updateIntervalHours > 0 {
		h.Set("profile-update-interval", fmt.Sprintf("%d", updateIntervalHours))
	}
	if title != "" {
		// Clash/Hiddify accept either a literal title or "base64:<b64>".
		// Use base64 so titles with non-ASCII (Persian names) round-trip
		// through the header parser cleanly.
		h.Set("profile-title", "base64:"+base64.StdEncoding.EncodeToString([]byte(title)))
	}
	if webPageURL != "" {
		h.Set("profile-web-page-url", webPageURL)
	}
	h.Set("Cache-Control", "no-store")
}

// NegotiateFormat picks an output format from the request's Accept header
// and User-Agent. Priority:
//  1. explicit ?format=clash|singbox|base64 query param wins (override)
//  2. User-Agent contains clash / mihomo → Clash
//  3. User-Agent contains sing-box / hiddify → sing-box
//  4. Accept header asks for json → sing-box, yaml → Clash
//  5. fallback: base64 (works everywhere)
func NegotiateFormat(r *http.Request) Format {
	if q := strings.ToLower(r.URL.Query().Get("format")); q != "" {
		switch q {
		case "clash", "yaml":
			return FormatClash
		case "singbox", "sing-box", "json":
			return FormatSingBox
		case "base64", "text", "v2ray":
			return FormatBase64
		}
	}
	ua := strings.ToLower(r.Header.Get("User-Agent"))
	switch {
	case strings.Contains(ua, "clash"), strings.Contains(ua, "mihomo"), strings.Contains(ua, "stash"):
		return FormatClash
	case strings.Contains(ua, "sing-box"), strings.Contains(ua, "hiddify"), strings.Contains(ua, "nekobox"), strings.Contains(ua, "nekoray"):
		return FormatSingBox
	}
	accept := strings.ToLower(r.Header.Get("Accept"))
	switch {
	case strings.Contains(accept, "application/json"):
		return FormatSingBox
	case strings.Contains(accept, "yaml"):
		return FormatClash
	}
	return FormatBase64
}
