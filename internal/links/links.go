package links

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
)

type Link struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// brandPrefix is the display brand shown at the start of every link name.
// Kept short so client UIs (v2rayN, Hiddify) don't truncate the meaningful
// suffix. Change here to rebrand everywhere.
const brandPrefix = "ZeroOne"

// linkName builds a clean display name for a generated VLESS share link.
// Email is intentionally not included — link fragments shouldn't leak the
// user's identity to anyone who screenshots a client. The name is stable
// across user renames too. Endpoint names are pretty-printed (kebab →
// title) so a config-named "runflare-xhttp-auto" reads as
// "Runflare Xhttp Auto" rather than the raw kebab.
func linkName(parts ...string) string {
	out := brandPrefix
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out += " · " + prettifyName(p)
	}
	return out
}

// prettifyName turns kebab/lower text into Title Case while preserving
// well-known acronyms (WS, XHTTP, etc.) and brand fragments (Runflare,
// Pars-Pack) that should keep their original casing. Hyphens become spaces.
func prettifyName(s string) string {
	if s == "" {
		return s
	}
	tokens := strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '_' || r == ' ' })
	for i, t := range tokens {
		lower := strings.ToLower(t)
		switch lower {
		case "ws":
			tokens[i] = "WS"
		case "xhttp":
			tokens[i] = "XHTTP"
		case "splithttp":
			tokens[i] = "SplitHTTP"
		case "tcp", "tls", "udp", "http":
			tokens[i] = strings.ToUpper(lower)
		default:
			if r := []rune(lower); len(r) > 0 {
				r[0] = unicode.ToUpper(r[0])
				tokens[i] = string(r)
			}
		}
	}
	return strings.Join(tokens, " ")
}

func VLESS(cfg stack.Config, user stack.User) []Link {
	if !user.Enabled && user.BannedUntil == 0 {
		return nil
	}
	host := cfg.Server.PublicIP
	limited := user.BandwidthPort > 0 && (user.DownloadMbps > 0 || user.UploadMbps > 0)
	if limited {
		return []Link{
			vlessLink(linkName("Limited"), user.UUID, host, user.BandwidthPort, "ws", "/limited/"+user.Email, "", false, stack.XHTTPClientExtra{}),
		}
	}
	out := []Link{
		vlessLink(linkName("Direct", "WS"), user.UUID, host, cfg.Xray.Inbounds.VLESSWSPort, "ws", "/vless", "", false, stack.XHTTPClientExtra{}),
	}
	if cfg.Xray.Inbounds.VLESSXHTTPPort > 0 {
		path := cfg.Xray.Inbounds.EffectiveVLESSXHTTPPath()
		mode := cfg.Xray.Inbounds.EffectiveVLESSXHTTPMode()
		out = append(out, vlessLink(linkName("Direct", "XHTTP"), user.UUID, host, 80, "xhttp", path, mode, false, stack.XHTTPClientExtra{}))
		if cfg.Xray.Inbounds.VLESSXHTTPLinkCompat {
			out = append(out, splitHTTPCompatLink(linkName("Direct", "SplitHTTP"), user.UUID, host, 80, path, mode, false))
		}
	}
	for _, ep := range cfg.Server.ClientEndpoints {
		if !ep.Enabled {
			continue
		}
		name := linkName(ep.Name)
		mode := ep.Mode
		if ep.Network == "xhttp" && mode == "" {
			mode = cfg.Xray.Inbounds.EffectiveVLESSXHTTPMode()
		}
		out = append(out, vlessLink(name, user.UUID, ep.Host, ep.Port, ep.Network, ep.Path, mode, ep.TLS, ep.Extra))
		if ep.Network == "xhttp" && ep.LinkCompat {
			out = append(out, splitHTTPCompatLink(linkName(ep.Name, "SplitHTTP"), user.UUID, ep.Host, ep.Port, ep.Path, mode, ep.TLS))
		}
	}
	return out
}

func SOCKS(cfg stack.Config, socks stack.SOCKSInbound) []Link {
	host := cfg.Server.PublicIP
	auth := url.QueryEscape(socks.Username) + ":" + url.QueryEscape(socks.Password)
	name := url.QueryEscape(socks.Username + "-socks")
	return []Link{
		{Name: "SOCKS5 URI", URL: fmt.Sprintf("socks5://%s@%s:%d#%s", auth, host, socks.Port, name)},
		{Name: "Host", URL: fmt.Sprintf("%s:%d", host, socks.Port)},
		{Name: "Username", URL: socks.Username},
		{Name: "Password", URL: socks.Password},
	}
}

func vlessLink(name, uuid, host string, port int, network, path, mode string, tls bool, extra stack.XHTTPClientExtra) Link {
	values := url.Values{}
	values.Set("encryption", "none")
	values.Set("type", network)
	if tls {
		values.Set("security", "tls")
		values.Set("sni", host)
		values.Set("host", host)
	} else {
		values.Set("security", "none")
	}
	values.Set("path", path)
	if network == "xhttp" && mode != "" {
		values.Set("mode", mode)
	}
	if network == "xhttp" {
		if blob := encodeXHTTPExtra(extra); blob != "" {
			values.Set("extra", blob)
		}
	}
	return Link{Name: name, URL: fmt.Sprintf("vless://%s@%s:%d?%s#%s", uuid, host, port, values.Encode(), url.QueryEscape(name))}
}

// splitHTTPCompatLink emits the Shadowrocket-friendly variant of an xhttp
// endpoint. Differences vs. the xray-native xhttp link:
//   - type=splithttp (legacy keyword Shadowrocket still recognizes)
//   - host= is always set, even without TLS — Shadowrocket uses it as the
//     HTTP Host header rather than defaulting to the connect target
//   - extra= is omitted; older Shadowrocket builds reject unknown params
func splitHTTPCompatLink(name, uuid, host string, port int, path, mode string, tls bool) Link {
	values := url.Values{}
	values.Set("encryption", "none")
	values.Set("type", "splithttp")
	if tls {
		values.Set("security", "tls")
		values.Set("sni", host)
	} else {
		values.Set("security", "none")
	}
	values.Set("host", host)
	values.Set("path", path)
	if mode != "" {
		values.Set("mode", mode)
	}
	return Link{Name: name, URL: fmt.Sprintf("vless://%s@%s:%d?%s#%s", uuid, host, port, values.Encode(), url.QueryEscape(name))}
}

// encodeXHTTPExtra renders the client xhttp tuning as a compact JSON
// object using xray's camelCase keys. Returns "" when no fields are
// set so the caller omits the URL parameter entirely.
func encodeXHTTPExtra(e stack.XHTTPClientExtra) string {
	obj := map[string]any{}
	if v := xhttpRangeOrInt(e.XPaddingBytes); v != nil {
		obj["xPaddingBytes"] = v
	}
	if e.NoSSEHeader != nil {
		obj["noSSEHeader"] = *e.NoSSEHeader
	}
	if e.NoGRPCHeader != nil {
		obj["noGRPCHeader"] = *e.NoGRPCHeader
	}
	if e.Xmux != nil {
		mux := map[string]any{}
		if v := xhttpRangeOrInt(e.Xmux.MaxConcurrency); v != nil {
			mux["maxConcurrency"] = v
		}
		if v := xhttpRangeOrInt(e.Xmux.MaxConnections); v != nil {
			mux["maxConnections"] = v
		}
		if e.Xmux.CMaxReuseTimes > 0 {
			mux["cMaxReuseTimes"] = e.Xmux.CMaxReuseTimes
		}
		if e.Xmux.CMaxLifetimeMs > 0 {
			mux["cMaxLifetimeMs"] = e.Xmux.CMaxLifetimeMs
		}
		if e.Xmux.HMaxRequestTimes > 0 {
			mux["hMaxRequestTimes"] = e.Xmux.HMaxRequestTimes
		}
		if len(mux) > 0 {
			obj["xmux"] = mux
		}
	}
	if len(obj) == 0 {
		return ""
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return ""
	}
	return string(b)
}

func xhttpRangeOrInt(s string) any {
	if s == "" {
		return nil
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return s
}
