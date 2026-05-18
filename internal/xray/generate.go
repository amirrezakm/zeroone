package xray

import (
	"net"
	"strconv"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
)

type Object = map[string]any

func Generate(cfg stack.Config) Object {
	unlimited := make([]Object, 0, len(cfg.Xray.Users))
	for _, u := range cfg.Xray.Users {
		if !u.Enabled {
			continue
		}
		if u.BandwidthPort > 0 && (u.DownloadMbps > 0 || u.UploadMbps > 0) {
			continue
		}
		unlimited = append(unlimited, Object{"id": u.UUID, "email": u.Email})
	}
	inbounds := []Object{
		vlessWSInbound("vless-public", cfg.Xray.Inbounds.VLESSWSPort, "0.0.0.0", unlimited, "/vless", true),
	}
	for _, socks := range cfg.Xray.Inbounds.PublicSOCKS {
		inbounds = append(inbounds, socksInbound(socks))
	}
	for _, u := range cfg.Xray.Users {
		if u.Enabled && u.BandwidthPort > 0 && (u.DownloadMbps > 0 || u.UploadMbps > 0) {
			inbounds = append(inbounds, limitedVLESSInbound(u))
		}
	}
	inbounds = append(inbounds,
		localSOCKSInbound(cfg.Xray.Inbounds.LocalSOCKSPort),
		vlessXHTTPInbound("vless-xhttp-local", cfg.Xray.Inbounds.VLESSXHTTPPort, "127.0.0.1", unlimited, cfg.Xray.Inbounds.EffectiveVLESSXHTTPPath(), cfg.Xray.Inbounds.EffectiveVLESSXHTTPMode(), cfg.Xray.Inbounds.VLESSXHTTPTuning),
		apiInbound(cfg.Xray.APIPort),
	)
	outbounds := []Object{vlessOutbound(cfg.Xray.Outbounds.Proxy), vlessOutbound(cfg.Xray.Outbounds.Fallback), directOutbound(), blockOutbound()}
	if rly := relayOutbound(cfg.Relay); rly != nil {
		outbounds = append(outbounds, rly)
	}
	return Object{
		"log":   Object{"loglevel": cfg.Xray.LogLevel},
		"dns":   Object{"hosts": cfg.Xray.DNSHosts, "queryStrategy": "UseIPv4", "servers": cfg.Xray.DNSServers},
		"api":   Object{"services": []string{"StatsService"}, "tag": "api"},
		"stats": Object{},
		"policy": Object{
			"levels": Object{"0": Object{"statsUserUplink": true, "statsUserDownlink": true}},
			"system": Object{
				"statsInboundUplink":    true,
				"statsInboundDownlink":  true,
				"statsOutboundUplink":   true,
				"statsOutboundDownlink": true,
			},
		},
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"routing":   Object{"domainStrategy": "IPIfNonMatch", "rules": routingRules(cfg)},
	}
}

// sniffing returns Xray's sniffing block. routeOnly=true keeps the sniffed
// SNI/Host for routing decisions but forwards the original destination,
// avoiding a redundant DNS resolution on the outbound.
func sniffing() Object {
	return Object{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "routeOnly": true}
}

func vlessWSInbound(tag string, port int, listen string, clients []Object, path string, fastOpen bool) Object {
	stream := Object{"network": "ws", "wsSettings": Object{"path": path}}
	if fastOpen {
		stream["sockopt"] = Object{"tcpFastOpen": true}
	}
	in := Object{"port": port, "listen": listen, "protocol": "vless", "settings": Object{"clients": clients, "decryption": "none"}, "streamSettings": stream, "sniffing": sniffing()}
	if tag != "" {
		in["tag"] = tag
	}
	return in
}

func vlessXHTTPInbound(tag string, port int, listen string, clients []Object, path, mode string, tuning stack.XHTTPInboundTuning) Object {
	settings := Object{"path": path}
	if mode != "" {
		settings["mode"] = mode
	}
	applyXHTTPInboundTuning(settings, tuning)
	in := Object{"port": port, "listen": listen, "protocol": "vless", "settings": Object{"clients": clients, "decryption": "none"}, "streamSettings": Object{"network": "xhttp", "xhttpSettings": settings}, "sniffing": sniffing()}
	if tag != "" {
		in["tag"] = tag
	}
	return in
}

// applyXHTTPInboundTuning merges optional xhttp tuning knobs onto the
// xhttpSettings object. Translates the stack.json snake_case to xray's
// camelCase. Range-or-int fields prefer integer when the value fits;
// otherwise they pass through as the original string so xray sees the
// range form ("100-1000") verbatim.
func applyXHTTPInboundTuning(settings Object, t stack.XHTTPInboundTuning) {
	if v := rangeOrInt(t.XPaddingBytes); v != nil {
		settings["xPaddingBytes"] = v
	}
	if t.ScMaxBufferedPosts > 0 {
		settings["scMaxBufferedPosts"] = t.ScMaxBufferedPosts
	}
	if v := rangeOrInt(t.ScMaxEachPostBytes); v != nil {
		settings["scMaxEachPostBytes"] = v
	}
	if v := rangeOrInt(t.ScStreamUpServerSecs); v != nil {
		settings["scStreamUpServerSecs"] = v
	}
	if t.KeepAlivePeriod > 0 {
		settings["keepAlivePeriod"] = t.KeepAlivePeriod
	}
	if t.NoSSEHeader != nil {
		settings["noSSEHeader"] = *t.NoSSEHeader
	}
	if t.NoGRPCHeader != nil {
		settings["noGRPCHeader"] = *t.NoGRPCHeader
	}
}

// rangeOrInt converts a stack.json range-or-int string into the form
// xray expects: a bare int ("1000000" → 1000000) when possible, the
// original range string otherwise ("100-1000" stays a string). Returns
// nil for empty input so the caller can omit the key entirely.
func rangeOrInt(s string) any {
	if s == "" {
		return nil
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return s
}

func socksInbound(s stack.SOCKSInbound) Object {
	return Object{"tag": "managed-socks-" + s.Name, "port": s.Port, "listen": s.Listen, "protocol": "socks", "settings": Object{"auth": "password", "accounts": []Object{{"user": s.Username, "pass": s.Password}}, "udp": true}, "sniffing": sniffing()}
}

func limitedVLESSInbound(u stack.User) Object {
	return Object{
		"tag":      "limited-" + u.Email,
		"port":     u.BandwidthPort,
		"listen":   "0.0.0.0",
		"protocol": "vless",
		"settings": Object{
			"clients":    []Object{{"id": u.UUID, "email": u.Email}},
			"decryption": "none",
		},
		"streamSettings": Object{"network": "ws", "security": "none", "wsSettings": Object{"path": "/limited/" + u.Email}, "sockopt": Object{"tcpFastOpen": true}},
		"sniffing":       sniffing(),
	}
}

func localSOCKSInbound(port int) Object {
	if port == 0 {
		port = 10808
	}
	return Object{"port": port, "listen": "127.0.0.1", "protocol": "socks", "settings": Object{"auth": "noauth", "udp": true}, "sniffing": sniffing()}
}

func apiInbound(port int) Object {
	if port == 0 {
		port = 10085
	}
	return Object{"tag": "xray-api", "listen": "127.0.0.1", "port": port, "protocol": "dokodemo-door", "settings": Object{"address": "127.0.0.1"}}
}

func vlessOutbound(o stack.Outbound) Object {
	stream := Object{"network": "ws", "wsSettings": Object{"path": o.Path, "host": o.Host}}
	if o.Type == "vless-ws-tls" {
		stream["security"] = "tls"
		stream["tlsSettings"] = Object{"serverName": o.ServerName, "fingerprint": "chrome"}
	} else {
		stream["security"] = "none"
	}
	sockopt := Object{"tcpFastOpen": true}
	if o.Interface != "" {
		sockopt["interface"] = o.Interface
	}
	stream["sockopt"] = sockopt
	out := Object{"tag": o.Tag, "protocol": "vless", "settings": Object{"vnext": []Object{{"address": o.Address, "port": o.Port, "users": []Object{{"id": o.UUID, "encryption": "none"}}}}}, "streamSettings": stream}
	if o.MuxConcurrency > 0 {
		// xudpProxyUDP443=skip lets QUIC/HTTP3 fall through to direct instead
		// of being rejected, which previously forced TCP fallback for many CDNs.
		out["mux"] = Object{"enabled": true, "concurrency": o.MuxConcurrency, "xudpConcurrency": o.MuxConcurrency, "xudpProxyUDP443": "skip"}
	}
	return out
}

func directOutbound() Object {
	return Object{"tag": "direct", "protocol": "freedom", "settings": Object{"domainStrategy": "UseIPv4"}, "streamSettings": Object{"sockopt": Object{"interface": "eth0", "tcpFastOpen": true}}}
}
func blockOutbound() Object { return Object{"tag": "block", "protocol": "blackhole"} }

// relayOutbound emits an HTTP outbound pointing at the local mhrv-rs proxy
// when the relay plugin is enabled and at least one routing target exists
// (either domain-based sites or whole xray inbounds via inbound_tags).
// Returns nil to omit the outbound entirely so existing traffic is
// untouched.
func relayOutbound(r stack.RelayConfig) Object {
	if !r.Enabled {
		return nil
	}
	if len(r.EnabledSites()) == 0 && len(r.InboundTags) == 0 {
		return nil
	}
	host, port, ok := parseRelayListen(r.EffectiveListen())
	if !ok {
		return nil
	}
	return Object{
		"tag":      r.EffectiveOutboundTag(),
		"protocol": "http",
		"settings": Object{
			"servers": []Object{{"address": host, "port": port}},
		},
		"streamSettings": Object{"sockopt": Object{"tcpFastOpen": true}},
	}
}

func parseRelayListen(addr string) (string, int, bool) {
	h, p, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, false
	}
	if h == "" || h == "0.0.0.0" {
		h = "127.0.0.1"
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		return "", 0, false
	}
	return h, n, true
}

func routingRules(cfg stack.Config) []Object {
	r := []Object{{"type": "field", "inboundTag": []string{"xray-api"}, "outboundTag": "api"}}
	if cfg.Xray.Routing.BlockUDP443 {
		r = append(r, Object{"type": "field", "network": "udp", "port": "443", "outboundTag": "block"})
	}
	if len(cfg.Xray.Routing.DirectDomains) > 0 {
		r = append(r, Object{"type": "field", "domain": cfg.Xray.Routing.DirectDomains, "outboundTag": "direct"})
	}
	if len(cfg.Xray.Routing.DirectIPs) > 0 {
		r = append(r, Object{"type": "field", "ip": cfg.Xray.Routing.DirectIPs, "outboundTag": "direct"})
	}
	if len(cfg.Xray.Routing.BlockDomains) > 0 {
		r = append(r, Object{"type": "field", "domain": cfg.Xray.Routing.BlockDomains, "outboundTag": "block"})
	}
	if len(cfg.Xray.Routing.BlockIPs) > 0 {
		r = append(r, Object{"type": "field", "ip": cfg.Xray.Routing.BlockIPs, "outboundTag": "block"})
	}
	if len(cfg.Xray.Routing.ManualBlockDomains) > 0 {
		r = append(r, Object{"type": "field", "domain": cfg.Xray.Routing.ManualBlockDomains, "outboundTag": "block"})
	}
	aiOutboundTag := cfg.Xray.Routing.AIOutboundTag
	if aiOutboundTag == "" {
		aiOutboundTag = cfg.Xray.Outbounds.Proxy.Tag
	}
	if len(cfg.Xray.Routing.AIUpdateDomains) > 0 {
		r = append(r, Object{"type": "field", "domain": cfg.Xray.Routing.AIUpdateDomains, "outboundTag": aiOutboundTag})
	}
	if len(cfg.Xray.Routing.AIDomains) > 0 {
		r = append(r, Object{"type": "field", "domain": cfg.Xray.Routing.AIDomains, "outboundTag": aiOutboundTag})
	}
	if cfg.Relay.Enabled {
		tag := cfg.Relay.EffectiveOutboundTag()
		// inbound_tags routes ALL traffic from those xray inbounds through
		// the relay — used when an entire dedicated inbound is the relay
		// frontend (e.g. an auth-gated SOCKS5 for personal backup use).
		if len(cfg.Relay.InboundTags) > 0 {
			r = append(r, Object{
				"type":        "field",
				"inboundTag":  append([]string(nil), cfg.Relay.InboundTags...),
				"outboundTag": tag,
			})
		}
		if domains := cfg.Relay.EnabledDomains(); len(domains) > 0 {
			r = append(r, Object{
				"type":        "field",
				"domain":      domains,
				"outboundTag": tag,
			})
		}
	}
	return r
}
