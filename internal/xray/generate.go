package xray

import "github.com/sakhtar/xray-stack-zeroone/internal/stack"

type Object = map[string]any

func Generate(cfg stack.Config) Object {
	clients := make([]Object, 0, len(cfg.Xray.Users))
	for _, u := range cfg.Xray.Users {
		if !u.Enabled {
			continue
		}
		clients = append(clients, Object{"id": u.UUID, "email": u.Email})
	}
	inbounds := []Object{
		vlessWSInbound(cfg.Xray.Inbounds.VLESSWSPort, "0.0.0.0", clients, "/vless"),
		vlessXHTTPInbound(cfg.Xray.Inbounds.VLESSXHTTPPort, "127.0.0.1", clients, "/xhttp"),
		apiInbound(cfg.Xray.APIPort),
	}
	for _, socks := range cfg.Xray.Inbounds.PublicSOCKS {
		inbounds = append(inbounds, socksInbound(socks))
	}
	return Object{
		"log":       Object{"loglevel": cfg.Xray.LogLevel},
		"dns":       Object{"hosts": cfg.Xray.DNSHosts, "queryStrategy": "UseIPv4", "servers": cfg.Xray.DNSServers},
		"api":       Object{"services": []string{"StatsService"}, "tag": "api"},
		"stats":     Object{},
		"policy":    Object{"levels": Object{"0": Object{"statsUserUplink": true, "statsUserDownlink": true}}, "system": Object{"statsInboundUplink": true, "statsInboundDownlink": true, "statsOutboundUplink": true, "statsOutboundDownlink": true}},
		"inbounds":  inbounds,
		"outbounds": []Object{vlessOutbound(cfg.Xray.Outbounds.Proxy), vlessOutbound(cfg.Xray.Outbounds.Fallback), directOutbound(), blockOutbound()},
		"routing":   Object{"domainStrategy": "IPIfNonMatch", "rules": routingRules(cfg)},
	}
}

func sniffing() Object {
	return Object{"enabled": true, "destOverride": []string{"http", "tls", "quic"}}
}

func vlessWSInbound(port int, listen string, clients []Object, path string) Object {
	return Object{"port": port, "listen": listen, "protocol": "vless", "settings": Object{"clients": clients, "decryption": "none"}, "streamSettings": Object{"network": "ws", "wsSettings": Object{"path": path}}, "sniffing": sniffing()}
}

func vlessXHTTPInbound(port int, listen string, clients []Object, path string) Object {
	return Object{"tag": "vless-xhttp-local", "port": port, "listen": listen, "protocol": "vless", "settings": Object{"clients": clients, "decryption": "none"}, "streamSettings": Object{"network": "xhttp", "xhttpSettings": Object{"path": path}}, "sniffing": sniffing()}
}

func socksInbound(s stack.SOCKSInbound) Object {
	return Object{"tag": "managed-socks-" + s.Name, "port": s.Port, "listen": s.Listen, "protocol": "socks", "settings": Object{"auth": "password", "accounts": []Object{{"user": s.Username, "pass": s.Password}}, "udp": true}, "sniffing": sniffing()}
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
	if o.Interface != "" {
		stream["sockopt"] = Object{"interface": o.Interface}
	}
	out := Object{"tag": o.Tag, "protocol": "vless", "settings": Object{"vnext": []Object{{"address": o.Address, "port": o.Port, "users": []Object{{"id": o.UUID, "encryption": "none"}}}}}, "streamSettings": stream}
	if o.MuxConcurrency > 0 {
		out["mux"] = Object{"enabled": true, "concurrency": o.MuxConcurrency, "xudpConcurrency": o.MuxConcurrency, "xudpProxyUDP443": "reject"}
	}
	return out
}

func directOutbound() Object {
	return Object{"tag": "direct", "protocol": "freedom", "streamSettings": Object{"sockopt": Object{"interface": "eth0"}}}
}
func blockOutbound() Object { return Object{"tag": "block", "protocol": "blackhole"} }

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
	if len(cfg.Xray.Routing.AIDomains) > 0 {
		r = append(r, Object{"type": "field", "domain": cfg.Xray.Routing.AIDomains, "outboundTag": cfg.Xray.Outbounds.Proxy.Tag})
	}
	return r
}
