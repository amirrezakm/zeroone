package stack

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Server   ServerConfig   `json:"server"`
	Xray     XrayConfig     `json:"xray"`
	Tunnels  []TunnelConfig `json:"tunnels"`
	Failover FailoverConfig `json:"failover"`
}

type ServerConfig struct {
	PublicIP            string `json:"public_ip"`
	AdminListen         string `json:"admin_listen"`
	XrayConfigPath      string `json:"xray_config_path"`
	XrayBinary          string `json:"xray_binary"`
	BackupDir           string `json:"backup_dir"`
	UserUsagePath       string `json:"user_usage_path"`
	SocksUsagePath      string `json:"socks_usage_path"`
	UIPath              string `json:"ui_path,omitempty"`
	BandwidthDevice     string `json:"bandwidth_device,omitempty"`
	BandwidthConfigPath string `json:"bandwidth_config_path,omitempty"`
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
	LocalSOCKSPort int            `json:"local_socks_port"`
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
	Email         string `json:"email"`
	UUID          string `json:"uuid"`
	Enabled       bool   `json:"enabled"`
	BannedUntil   int64  `json:"banned_until,omitempty"`
	QuotaBytes    int64  `json:"quota_bytes,omitempty"`
	DownloadMbps  int    `json:"download_mbps,omitempty"`
	UploadMbps    int    `json:"upload_mbps,omitempty"`
	BandwidthPort int    `json:"bandwidth_port,omitempty"`
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
	BlockUDP443        bool     `json:"block_udp_443"`
	DirectDomains      []string `json:"direct_domains"`
	DirectIPs          []string `json:"direct_ips"`
	BlockDomains       []string `json:"block_domains"`
	ManualBlockDomains []string `json:"manual_block_domains"`
	BlockIPs           []string `json:"block_ips"`
	AIUpdateDomains    []string `json:"ai_update_domains"`
	AIDomains          []string `json:"ai_domains"`
	AIOutboundTag      string   `json:"ai_outbound_tag,omitempty"`
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

func Save(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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
	ports := map[int]string{}
	addPort := func(port int, owner string) error {
		if port == 0 {
			return nil
		}
		if existing := ports[port]; existing != "" {
			return fmt.Errorf("port %d is used by both %q and %q", port, existing, owner)
		}
		ports[port] = owner
		return nil
	}
	for _, item := range []struct {
		port  int
		owner string
	}{
		{c.Xray.Inbounds.VLESSWSPort, "vless-ws"},
		{c.Xray.Inbounds.VLESSXHTTPPort, "vless-xhttp"},
		{c.Xray.Inbounds.LocalSOCKSPort, "local-socks"},
		{c.Xray.APIPort, "xray-api"},
	} {
		if err := addPort(item.port, item.owner); err != nil {
			return err
		}
	}
	for _, u := range c.Xray.Users {
		if u.Email == "" || u.UUID == "" {
			return fmt.Errorf("users need email and uuid")
		}
		if seen[u.Email] {
			return fmt.Errorf("duplicate user email %q", u.Email)
		}
		seen[u.Email] = true
		if u.QuotaBytes < 0 || u.DownloadMbps < 0 || u.UploadMbps < 0 {
			return fmt.Errorf("negative limits are not valid for user %q", u.Email)
		}
		if u.BandwidthPort != 0 {
			if u.BandwidthPort < 1 || u.BandwidthPort > 65535 {
				return fmt.Errorf("invalid bandwidth port for user %q", u.Email)
			}
			if err := addPort(u.BandwidthPort, "bandwidth "+u.Email); err != nil {
				return err
			}
		}
	}
	for _, s := range c.Xray.Inbounds.PublicSOCKS {
		if err := addPort(s.Port, "SOCKS "+s.Name); err != nil {
			return err
		}
	}
	return nil
}
