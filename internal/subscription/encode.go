// Package subscription renders per-user subscription content in the
// formats VPN clients expect: a base64-encoded list of vless:// URIs
// (the universal baseline), Clash YAML, and sing-box JSON.
//
// Content negotiation lives in the HTTP handler (server.go); the encoders
// here are pure functions over already-generated link lists so they're
// trivially testable.
package subscription

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/sakhtar/xray-stack-zeroone/internal/links"
)

// Format names the wire format requested by the client.
type Format int

const (
	FormatBase64  Format = iota // base64(text/plain "vless://...\n...")
	FormatClash                 // YAML, proxies block + proxy-groups
	FormatSingBox               // JSON, outbounds array
)

// String returns the canonical name for logging/Content-Type negotiation.
func (f Format) String() string {
	switch f {
	case FormatClash:
		return "clash"
	case FormatSingBox:
		return "sing-box"
	default:
		return "base64"
	}
}

// ContentType returns the MIME type to return for this format.
func (f Format) ContentType() string {
	switch f {
	case FormatClash:
		return "application/x-yaml; charset=utf-8"
	case FormatSingBox:
		return "application/json; charset=utf-8"
	default:
		return "text/plain; charset=utf-8"
	}
}

// Encode serializes the given links into the requested format. Returns
// the body bytes and the Content-Type to set on the response.
func Encode(format Format, all []links.Link) ([]byte, string) {
	switch format {
	case FormatClash:
		return EncodeClash(all), format.ContentType()
	case FormatSingBox:
		return EncodeSingBox(all), format.ContentType()
	default:
		return EncodeBase64(all), format.ContentType()
	}
}

// EncodeBase64 returns "vless://...\nvless://...\n..." base64-encoded.
// This is what classic v2rayN / V2RayNG / Streisand expect. Modern clients
// (Hiddify, sing-box, NekoBox) also accept it as a fallback.
func EncodeBase64(all []links.Link) []byte {
	var buf strings.Builder
	for _, l := range all {
		buf.WriteString(l.URL)
		buf.WriteByte('\n')
	}
	// Standard padding-included base64; some clients reject URL-safe variant.
	enc := base64.StdEncoding.EncodeToString([]byte(buf.String()))
	return []byte(enc)
}

// EncodeClash renders a minimal Clash Meta / Mihomo YAML with a `proxies:`
// list and a single `proxy-groups:` selector named "ZeroOne" so the user
// can pick which edge they want. Each proxy entry is decoded from the
// vless:// URI into Clash's native VLESS schema.
//
// Only fields that meaningfully change client behavior are emitted. Clash
// Meta tolerates missing optional fields (it applies its own defaults),
// which keeps the YAML compact.
func EncodeClash(all []links.Link) []byte {
	var buf strings.Builder
	buf.WriteString("proxies:\n")
	names := make([]string, 0, len(all))
	for _, l := range all {
		p := parseVLESS(l.URL)
		if p == nil {
			continue
		}
		name := yamlString(l.Name)
		names = append(names, l.Name)
		fmt.Fprintf(&buf, "  - name: %s\n", name)
		fmt.Fprintf(&buf, "    type: vless\n")
		fmt.Fprintf(&buf, "    server: %s\n", yamlString(p.host))
		fmt.Fprintf(&buf, "    port: %d\n", p.port)
		fmt.Fprintf(&buf, "    uuid: %s\n", p.uuid)
		fmt.Fprintf(&buf, "    network: %s\n", clashNetwork(p.transport))
		fmt.Fprintf(&buf, "    udp: true\n")
		if p.tls {
			fmt.Fprintf(&buf, "    tls: true\n")
			if p.sni != "" {
				fmt.Fprintf(&buf, "    servername: %s\n", yamlString(p.sni))
			}
			fmt.Fprintf(&buf, "    client-fingerprint: chrome\n")
		}
		switch p.transport {
		case "ws":
			fmt.Fprintf(&buf, "    ws-opts:\n")
			fmt.Fprintf(&buf, "      path: %s\n", yamlString(p.path))
			if p.hostHeader != "" {
				fmt.Fprintf(&buf, "      headers:\n        Host: %s\n", yamlString(p.hostHeader))
			}
		case "xhttp", "splithttp":
			// Clash Meta calls xhttp "h2" + chunked or grpc — there is no
			// 1:1 mapping. Emit as h2 with a path (closest behavior for
			// the most common case). Older Clash builds will ignore the
			// entry, which is acceptable: xhttp users typically run
			// xray-core or sing-box where the base64/json formats apply.
			fmt.Fprintf(&buf, "    h2-opts:\n")
			fmt.Fprintf(&buf, "      path: %s\n", yamlString(p.path))
			if p.hostHeader != "" {
				fmt.Fprintf(&buf, "      host: [%s]\n", yamlString(p.hostHeader))
			}
		}
	}
	buf.WriteString("\nproxy-groups:\n")
	buf.WriteString("  - name: ZeroOne\n")
	buf.WriteString("    type: select\n")
	buf.WriteString("    proxies:\n")
	for _, n := range names {
		fmt.Fprintf(&buf, "      - %s\n", yamlString(n))
	}
	buf.WriteString("\nrules:\n")
	buf.WriteString("  - MATCH,ZeroOne\n")
	return []byte(buf.String())
}

// EncodeSingBox returns a sing-box config with outbounds for every link
// plus a `selector` outbound named "ZeroOne" wired as the default route.
// This is what Hiddify and modern sing-box clients prefer.
func EncodeSingBox(all []links.Link) []byte {
	outbounds := []map[string]any{}
	tags := []string{}
	for _, l := range all {
		p := parseVLESS(l.URL)
		if p == nil {
			continue
		}
		tags = append(tags, l.Name)
		ob := map[string]any{
			"type":        "vless",
			"tag":         l.Name,
			"server":      p.host,
			"server_port": p.port,
			"uuid":        p.uuid,
			"flow":        "",
			"packet_encoding": "xudp",
		}
		switch p.transport {
		case "ws":
			ob["transport"] = map[string]any{
				"type":    "ws",
				"path":    p.path,
				"headers": headerMap(p.hostHeader),
			}
		case "xhttp", "splithttp":
			t := map[string]any{
				"type": "xhttp",
				"path": p.path,
			}
			if p.hostHeader != "" {
				t["host"] = []string{p.hostHeader}
			}
			if p.mode != "" {
				t["mode"] = p.mode
			}
			ob["transport"] = t
		}
		if p.tls {
			ob["tls"] = map[string]any{
				"enabled":     true,
				"server_name": p.sni,
				"utls": map[string]any{
					"enabled":     true,
					"fingerprint": "chrome",
				},
			}
		}
		outbounds = append(outbounds, ob)
	}
	// Selector outbound = user-pickable group. Routes default to it.
	outbounds = append(outbounds,
		map[string]any{"type": "selector", "tag": "ZeroOne", "outbounds": tags, "default": defaultTag(tags)},
		map[string]any{"type": "direct", "tag": "direct"},
		map[string]any{"type": "block", "tag": "block"},
	)
	cfg := map[string]any{
		"outbounds": outbounds,
		"route": map[string]any{
			"rules":  []map[string]any{{"protocol": "dns", "outbound": "direct"}},
			"final":  "ZeroOne",
			"auto_detect_interface": true,
		},
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return b
}

// parsed holds the subset of vless:// query params the encoders care about.
type parsed struct {
	uuid       string
	host       string
	port       int
	transport  string // ws | xhttp | splithttp | tcp
	path       string
	hostHeader string
	sni        string
	tls        bool
	mode       string
}

// parseVLESS extracts the fields we need from a vless:// URL. Returns nil
// on parse failure rather than erroring — the caller skips malformed
// entries silently so one bad link doesn't poison the whole subscription.
func parseVLESS(raw string) *parsed {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "vless" {
		return nil
	}
	portInt := 0
	if p := u.Port(); p != "" {
		portInt, _ = strconv.Atoi(p)
	}
	q := u.Query()
	tr := q.Get("type")
	if tr == "" {
		tr = "tcp"
	}
	return &parsed{
		uuid:       u.User.Username(),
		host:       u.Hostname(),
		port:       portInt,
		transport:  tr,
		path:       q.Get("path"),
		hostHeader: q.Get("host"),
		sni:        q.Get("sni"),
		tls:        q.Get("security") == "tls",
		mode:       q.Get("mode"),
	}
}

// yamlString quotes a value if it contains characters that would confuse
// a YAML scalar parser (colons, leading/trailing whitespace, etc.). Keeps
// the output readable for safe values, safe for unsafe ones.
func yamlString(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, ":#{}[]&*!|>'\"%@`,?\n\t") || strings.HasPrefix(s, "-") || strings.HasPrefix(s, " ") || strings.HasSuffix(s, " ") {
		return strconv.Quote(s)
	}
	return s
}

func clashNetwork(t string) string {
	switch t {
	case "ws":
		return "ws"
	case "xhttp", "splithttp":
		return "h2"
	default:
		return "tcp"
	}
}

func headerMap(host string) map[string]string {
	if host == "" {
		return map[string]string{}
	}
	return map[string]string{"Host": host}
}

func defaultTag(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	return tags[0]
}
