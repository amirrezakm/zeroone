package stack

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	Server   ServerConfig   `json:"server"`
	Xray     XrayConfig     `json:"xray"`
	Tunnels  []TunnelConfig `json:"tunnels"`
	Failover FailoverConfig `json:"failover"`
}

type ServerConfig struct {
	PublicIP       string `json:"public_ip"`
	AdminListen    string `json:"admin_listen"`
	XrayConfigPath string `json:"xray_config_path"`
	XrayBinary     string `json:"xray_binary"`
	BackupDir      string `json:"backup_dir"`
}

type XrayConfig struct {
	LogLevel   string              `json:"log_level"`
	DNSServers []string            `json:"dns_servers"`
	DNSHosts   map[string][]string `json:"dns_hosts"`
	APIPort    int                 `json:"api_port"`
	Inbounds   InboundConfig       `json:"inbounds"`
	Users      []User              `json:"users"`
	Outbounds  OutboundSet         `json:"outbounds"`
	Routing    RoutingConfig       `json:"routing"`
}

type InboundConfig struct {
	VLESSWSPort    int            `json:"vless_ws_port"`
	VLESSXHTTPPort int            `json:"vless_xhttp_port"`
	PublicSOCKS    []SOCKSInbound `json:"public_socks"`
}

type SOCKSInbound struct {
	Name     string `json:"name"`
	Listen   string `json:"listen"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type User struct {
	Email   string `json:"email"`
	UUID    string `json:"uuid"`
	Enabled bool   `json:"enabled"`
}

type OutboundSet struct {
	Proxy    Outbound `json:"proxy"`
	Fallback Outbound `json:"fallback"`
}

type Outbound struct {
	Tag            string `json:"tag"`
	Type           string `json:"type"`
	Address        string `json:"address"`
	Port           int    `json:"port"`
	UUID           string `json:"uuid"`
	ServerName     string `json:"server_name"`
	Host           string `json:"host"`
	Path           string `json:"path"`
	Interface      string `json:"interface,omitempty"`
	MuxConcurrency int    `json:"mux_concurrency,omitempty"`
}

type RoutingConfig struct {
	BlockUDP443   bool     `json:"block_udp_443"`
	DirectDomains []string `json:"direct_domains"`
	DirectIPs     []string `json:"direct_ips"`
	BlockDomains  []string `json:"block_domains"`
	BlockIPs      []string `json:"block_ips"`
	AIDomains     []string `json:"ai_domains"`
}

type TunnelConfig struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Interface   string `json:"interface"`
	SystemdUnit string `json:"systemd_unit"`
	Priority    int    `json:"priority"`
}

type FailoverConfig struct {
	Enabled             bool   `json:"enabled"`
	ProbeIP             string `json:"probe_ip"`
	ProbePort           int    `json:"probe_port"`
	IntervalSeconds     int    `json:"interval_seconds"`
	Confirmations       int    `json:"confirmations"`
	CooldownSeconds     int    `json:"cooldown_seconds"`
	FallbackOutboundTag string `json:"fallback_outbound_tag"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c Config) Validate() error {
	if c.Server.AdminListen == "" {
		return fmt.Errorf("server.admin_listen is required")
	}
	if c.Server.XrayConfigPath == "" {
		return fmt.Errorf("server.xray_config_path is required")
	}
	if c.Xray.Inbounds.VLESSWSPort == 0 {
		return fmt.Errorf("xray.inbounds.vless_ws_port is required")
	}
	if c.Xray.Outbounds.Proxy.Tag == "" {
		return fmt.Errorf("xray.outbounds.proxy.tag is required")
	}
	seen := map[string]bool{}
	for _, u := range c.Xray.Users {
		if !u.Enabled {
			continue
		}
		if u.Email == "" || u.UUID == "" {
			return fmt.Errorf("enabled users need email and uuid")
		}
		if seen[u.Email] {
			return fmt.Errorf("duplicate user email %q", u.Email)
		}
		seen[u.Email] = true
	}
	return nil
}
