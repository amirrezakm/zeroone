package stack

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Server   ServerConfig   `json:"server"`
	Xray     XrayConfig     `json:"xray"`
	Tunnels  []TunnelConfig `json:"tunnels"`
	Failover FailoverConfig `json:"failover"`
	Panel    PanelConfig    `json:"panel,omitempty"`
	Relay    RelayConfig    `json:"relay,omitempty"`
}

// RelayConfig wires the MasterHttpRelayVPN (mhrv-rs) plugin into the stack.
// When Enabled, xray gains an HTTP outbound pointing at Listen and selected
// Sites are routed to it. The supervisor optionally manages the mhrv-rs
// binary itself.
type RelayConfig struct {
	Enabled       bool              `json:"enabled"`
	Listen        string            `json:"listen,omitempty"`
	SOCKSPort     int               `json:"socks_port,omitempty"`
	AuthKey       string            `json:"auth_key,omitempty"`
	ScriptURL     string            `json:"script_url,omitempty"`
	DeploymentIDs []string          `json:"deployment_ids,omitempty"`
	GoogleIP      string            `json:"google_ip,omitempty"`
	FrontDomain   string            `json:"front_domain,omitempty"`
	LogLevel      string            `json:"log_level,omitempty"`
	VerifySSL     *bool             `json:"verify_ssl,omitempty"`
	Binary        string            `json:"binary,omitempty"`
	ConfigPath    string            `json:"config_path,omitempty"`
	SystemdUnit   string            `json:"systemd_unit,omitempty"`
	OutboundTag   string            `json:"outbound_tag,omitempty"`
	Sites         []RelaySite       `json:"sites,omitempty"`
	InboundTags   []string          `json:"inbound_tags,omitempty"`
	HostsMap      map[string]string `json:"hosts_map,omitempty"`
	StatePath     string            `json:"state_path,omitempty"`
	LogPath       string            `json:"log_path,omitempty"`
	HealthProbe   string            `json:"health_probe,omitempty"`
	Notes         string            `json:"notes,omitempty"`
	NoCertCheck   bool              `json:"no_cert_check,omitempty"`
}

type RelaySite struct {
	Domain  string `json:"domain"`
	Enabled bool   `json:"enabled"`
	Note    string `json:"note,omitempty"`
}

const (
	DefaultRelayListen      = "127.0.0.1:8085"
	DefaultRelaySOCKSPort   = 8086
	DefaultRelayOutboundTag = "relay-mhrv"
	DefaultRelayBinary      = "/usr/local/bin/mhrv-rs"
	DefaultRelayConfigPath  = "/var/lib/zeroone/relay/config.json"
	DefaultRelayStatePath   = "/var/lib/zeroone/relay/state.json"
	DefaultRelayLogPath     = "/var/lib/zeroone/relay/relay.log"
	DefaultRelayHealthProbe = "www.google.com:443"
	DefaultRelayGoogleIP    = "216.239.38.120"
	DefaultRelayFrontDomain = "www.google.com"
	DefaultRelayLogLevel    = "info"
)

// EffectiveListen returns the listen address with the default applied.
func (r RelayConfig) EffectiveListen() string {
	if r.Listen == "" {
		return DefaultRelayListen
	}
	return r.Listen
}

// EffectiveOutboundTag returns the outbound tag with the default applied.
func (r RelayConfig) EffectiveOutboundTag() string {
	if r.OutboundTag == "" {
		return DefaultRelayOutboundTag
	}
	return r.OutboundTag
}

// EnabledSites returns sites that should be routed through the relay.
func (r RelayConfig) EnabledSites() []RelaySite {
	out := make([]RelaySite, 0, len(r.Sites))
	for _, s := range r.Sites {
		if s.Enabled && s.Domain != "" {
			out = append(out, s)
		}
	}
	return out
}

// EnabledDomains returns the domain strings for sites currently routed
// through the relay (one entry per enabled site).
func (r RelayConfig) EnabledDomains() []string {
	sites := r.EnabledSites()
	out := make([]string, 0, len(sites))
	for _, s := range sites {
		out = append(out, s.Domain)
	}
	return out
}

type PanelConfig struct {
	Tokens        []APIToken          `json:"tokens,omitempty"`
	Admins        []Admin             `json:"admins,omitempty"`
	SessionSecret string              `json:"session_secret,omitempty"`
	Notifications NotificationsConfig `json:"notifications,omitempty"`
}

type APIToken struct {
	ID        string `json:"id"`
	Hash      string `json:"hash"`
	Scope     string `json:"scope,omitempty"`
	CreatedAt int64  `json:"created_at"`
	LastUsed  int64  `json:"last_used,omitempty"`
}

// Admin holds the credentials for a named panel administrator. The password
// is stored as a pbkdf2-sha256 hash in the format "pbkdf2-sha256$ITER$SALT$HASH"
// where SALT and HASH are base64-std-encoded. Plaintext is never stored.
type Admin struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	CreatedAt    int64  `json:"created_at"`
	LastLogin    int64  `json:"last_login,omitempty"`
}

type NotificationsConfig struct {
	Webhook  WebhookSink  `json:"webhook,omitempty"`
	Telegram TelegramSink `json:"telegram,omitempty"`
}

type WebhookSink struct {
	URL    string   `json:"url,omitempty"`
	Secret string   `json:"secret,omitempty"`
	Events []string `json:"events,omitempty"`
}

type TelegramSink struct {
	BotToken string   `json:"bot_token,omitempty"`
	ChatID   string   `json:"chat_id,omitempty"`
	Events   []string `json:"events,omitempty"`
}

type ServerConfig struct {
	PublicIP            string           `json:"public_ip"`
	ClientEndpoints     []ClientEndpoint `json:"client_endpoints,omitempty"`
	AdminListen         string           `json:"admin_listen"`
	XrayConfigPath      string           `json:"xray_config_path"`
	XrayBinary          string           `json:"xray_binary"`
	BackupDir           string           `json:"backup_dir"`
	UserUsagePath       string           `json:"user_usage_path"`
	SocksUsagePath      string           `json:"socks_usage_path"`
	UIPath              string           `json:"ui_path,omitempty"`
	BandwidthDevice     string           `json:"bandwidth_device,omitempty"`
	BandwidthConfigPath string           `json:"bandwidth_config_path,omitempty"`
	FailoverStatePath   string           `json:"failover_state_path,omitempty"`
	FailoverHistoryPath string           `json:"failover_history_path,omitempty"`
	DestinationsPath    string           `json:"destinations_path,omitempty"`
}

type ClientEndpoint struct {
	Name       string           `json:"name"`
	Host       string           `json:"host"`
	Port       int              `json:"port"`
	Network    string           `json:"network"`
	Path       string           `json:"path"`
	Mode       string           `json:"mode,omitempty"`
	TLS        bool             `json:"tls"`
	Enabled    bool             `json:"enabled"`
	Extra      XHTTPClientExtra `json:"extra,omitempty"`
	LinkCompat bool             `json:"link_compat,omitempty"`
}

// XHTTPClientExtra holds the client-side xhttp knobs that get serialized
// into the `extra=` query parameter of the generated VLESS share URL.
// Most modern clients (v2rayN, NekoBox, Hiddify) parse this and pass it
// directly to xray's xhttp outbound. Fields are translated to xray's
// camelCase names at emit time; leave any field zero/nil to omit.
type XHTTPClientExtra struct {
	XPaddingBytes string        `json:"x_padding_bytes,omitempty"`
	NoSSEHeader   *bool         `json:"no_sse_header,omitempty"`
	NoGRPCHeader  *bool         `json:"no_grpc_header,omitempty"`
	Xmux          *XmuxSettings `json:"xmux,omitempty"`
}

// XmuxSettings tunes xray's HTTP multiplexing for xhttp outbounds.
// CMaxLifetimeMs is the most important knob for CDN-fronted xhttp:
// many edges (WCDN, Cloudflare) reset idle connections at ~100s — set
// CMaxLifetimeMs below that so the client rotates first and avoids
// mid-stream resets.
type XmuxSettings struct {
	MaxConcurrency   string `json:"max_concurrency,omitempty"`
	MaxConnections   string `json:"max_connections,omitempty"`
	CMaxReuseTimes   int    `json:"c_max_reuse_times,omitempty"`
	CMaxLifetimeMs   int    `json:"c_max_lifetime_ms,omitempty"`
	HMaxRequestTimes int    `json:"h_max_request_times,omitempty"`
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
	VLESSWSPort          int                `json:"vless_ws_port"`
	VLESSXHTTPPort       int                `json:"vless_xhttp_port"`
	VLESSXHTTPPath       string             `json:"vless_xhttp_path,omitempty"`
	VLESSXHTTPMode       string             `json:"vless_xhttp_mode,omitempty"`
	VLESSXHTTPTuning     XHTTPInboundTuning `json:"vless_xhttp_tuning,omitempty"`
	VLESSXHTTPLinkCompat bool               `json:"vless_xhttp_link_compat,omitempty"`
	LocalSOCKSPort       int                `json:"local_socks_port"`
	PublicSOCKS          []SOCKSInbound     `json:"public_socks"`
}

// XHTTPInboundTuning carries the optional xhttp inbound knobs that xray
// exposes for stream-up/packet-up performance tuning and DPI cover. All
// fields are pass-through to xray's xhttpSettings (translated to
// camelCase at emit time); leave a field zero/nil to omit it.
//
// Typical values for AI/SSE workloads behind a CDN:
//
//	x_padding_bytes:          "100-1000"
//	sc_max_buffered_posts:    30
//	sc_max_each_post_bytes:   "1000000"
//	sc_stream_up_server_secs: "20-80"
//	keep_alive_period:        30
type XHTTPInboundTuning struct {
	XPaddingBytes        string `json:"x_padding_bytes,omitempty"`
	ScMaxBufferedPosts   int    `json:"sc_max_buffered_posts,omitempty"`
	ScMaxEachPostBytes   string `json:"sc_max_each_post_bytes,omitempty"`
	ScStreamUpServerSecs string `json:"sc_stream_up_server_secs,omitempty"`
	KeepAlivePeriod      int    `json:"keep_alive_period,omitempty"`
	NoSSEHeader          *bool  `json:"no_sse_header,omitempty"`
	NoGRPCHeader         *bool  `json:"no_grpc_header,omitempty"`
}

func (i InboundConfig) EffectiveVLESSXHTTPPath() string {
	if i.VLESSXHTTPPath == "" {
		return "/xhttp"
	}
	return i.VLESSXHTTPPath
}

func (i InboundConfig) EffectiveVLESSXHTTPMode() string {
	if i.VLESSXHTTPMode == "" {
		return "auto"
	}
	return i.VLESSXHTTPMode
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
	// Period quotas are sub-caps under QuotaBytes. Each is enforced
	// independently against the matching counter in usage.UserState and
	// reset on its own schedule (daily at DailyResetHHMM server-local;
	// weekly at Monday 00:00 local; monthly at the 1st 00:00 local).
	// Zero means no per-period cap for that period.
	DailyQuotaBytes   int64 `json:"daily_quota_bytes,omitempty"`
	WeeklyQuotaBytes  int64 `json:"weekly_quota_bytes,omitempty"`
	MonthlyQuotaBytes int64 `json:"monthly_quota_bytes,omitempty"`
	// DailyResetHHMM is the time of day (server-local) the daily counter
	// rolls over, in 24-hour "HH:MM" format. Empty defaults to "00:00".
	DailyResetHHMM string `json:"daily_reset_hhmm,omitempty"`
	// MaxSessions caps the number of distinct client IPs allowed to be
	// concurrently connected. The session enforcer kicks the
	// least-recently-seen IPs back down to MaxSessions. Zero means no cap.
	MaxSessions int `json:"max_sessions,omitempty"`
	// SubToken is a 32-hex-char opaque identifier the user passes in
	// /sub/{token} and /me/{token} URLs. Stable across config edits;
	// regenerating one invalidates the user's existing share/portal URLs.
	// Generated lazily by EnsureSubTokens on startup if missing.
	SubToken string `json:"sub_token,omitempty"`
}

// EffectiveDailyResetHHMM returns the user's daily reset time with
// the "00:00" default applied.
func (u User) EffectiveDailyResetHHMM() string {
	if u.DailyResetHHMM == "" {
		return "00:00"
	}
	return u.DailyResetHHMM
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

// FailoverMode controls how the failover loop picks an active tunnel.
//   - "" / "auto"     — health-based, walks Tunnels in order
//   - "manual"        — pin to PreferredTunnel; drift only if it goes unhealthy
//     (does not auto-return when it recovers; user re-picks)
//   - "preferred"     — same as manual but auto-returns to PreferredTunnel
//     once it becomes healthy again
const (
	FailoverModeAuto      = "auto"
	FailoverModeManual    = "manual"
	FailoverModePreferred = "preferred"
)

type FailoverConfig struct {
	Enabled             bool          `json:"enabled"`
	Mode                string        `json:"mode,omitempty"`
	PreferredTunnel     string        `json:"preferred_tunnel,omitempty"`
	ProbeIP             string        `json:"probe_ip"`
	ProbePort           int           `json:"probe_port"`
	Probes              []ProbeTarget `json:"probes,omitempty"`
	IntervalSeconds     int           `json:"interval_seconds"`
	Confirmations       int           `json:"confirmations"`
	CooldownSeconds     int           `json:"cooldown_seconds"`
	FallbackOutboundTag string        `json:"fallback_outbound_tag"`
}

// EffectiveMode returns the configured mode, defaulting to "auto" when unset.
func (f FailoverConfig) EffectiveMode() string {
	switch f.Mode {
	case FailoverModeManual, FailoverModePreferred:
		return f.Mode
	default:
		return FailoverModeAuto
	}
}

type ProbeTarget struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
}

func (f FailoverConfig) ProbeTargets() []ProbeTarget {
	if len(f.Probes) > 0 {
		out := make([]ProbeTarget, 0, len(f.Probes))
		for _, p := range f.Probes {
			if p.Address == "" {
				continue
			}
			if p.Port == 0 {
				p.Port = 443
			}
			out = append(out, p)
		}
		if len(out) > 0 {
			return out
		}
	}
	port := f.ProbePort
	if port == 0 {
		port = 443
	}
	if f.ProbeIP == "" {
		return []ProbeTarget{{Address: "1.1.1.1", Port: port}}
	}
	return []ProbeTarget{{Address: f.ProbeIP, Port: port}}
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
	if path := c.Xray.Inbounds.EffectiveVLESSXHTTPPath(); path == "" || path[0] != '/' {
		return fmt.Errorf("xray.inbounds.vless_xhttp_path must start with /")
	}
	switch c.Xray.Inbounds.EffectiveVLESSXHTTPMode() {
	case "auto", "packet-up", "stream-up", "stream-one":
	default:
		return fmt.Errorf("xray.inbounds.vless_xhttp_mode must be auto, packet-up, stream-up, or stream-one")
	}
	if err := c.Xray.Inbounds.VLESSXHTTPTuning.validate("xray.inbounds.vless_xhttp_tuning"); err != nil {
		return err
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
	seenEndpoint := map[string]bool{}
	for _, ep := range c.Server.ClientEndpoints {
		if ep.Name == "" {
			return fmt.Errorf("client endpoint name is required")
		}
		if seenEndpoint[ep.Name] {
			return fmt.Errorf("duplicate client endpoint %q", ep.Name)
		}
		seenEndpoint[ep.Name] = true
		if ep.Host == "" {
			return fmt.Errorf("client endpoint %q host is required", ep.Name)
		}
		if ep.Port < 1 || ep.Port > 65535 {
			return fmt.Errorf("client endpoint %q port is out of range", ep.Name)
		}
		if ep.Network != "ws" && ep.Network != "xhttp" {
			return fmt.Errorf("client endpoint %q network must be ws or xhttp", ep.Name)
		}
		if ep.Path == "" || ep.Path[0] != '/' {
			return fmt.Errorf("client endpoint %q path must start with /", ep.Name)
		}
		if ep.Mode != "" {
			switch ep.Mode {
			case "auto", "packet-up", "stream-up", "stream-one":
			default:
				return fmt.Errorf("client endpoint %q mode must be auto, packet-up, stream-up, or stream-one", ep.Name)
			}
		}
		if err := ep.Extra.validate(fmt.Sprintf("client endpoint %q extra", ep.Name)); err != nil {
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
		if u.DailyQuotaBytes < 0 || u.WeeklyQuotaBytes < 0 || u.MonthlyQuotaBytes < 0 {
			return fmt.Errorf("negative period quotas are not valid for user %q", u.Email)
		}
		if u.MaxSessions < 0 {
			return fmt.Errorf("max_sessions must be >= 0 for user %q", u.Email)
		}
		if u.DailyResetHHMM != "" {
			if _, _, err := ParseHHMM(u.DailyResetHHMM); err != nil {
				return fmt.Errorf("user %q daily_reset_hhmm: %w", u.Email, err)
			}
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
	seenAdmin := map[string]bool{}
	for _, a := range c.Panel.Admins {
		if a.Username == "" {
			return fmt.Errorf("admin username is required")
		}
		if seenAdmin[a.Username] {
			return fmt.Errorf("duplicate admin %q", a.Username)
		}
		seenAdmin[a.Username] = true
		if a.PasswordHash == "" {
			return fmt.Errorf("admin %q password_hash is required", a.Username)
		}
	}
	switch c.Failover.Mode {
	case "", FailoverModeAuto, FailoverModeManual, FailoverModePreferred:
	default:
		return fmt.Errorf("failover.mode must be auto, manual, or preferred")
	}
	if c.Failover.PreferredTunnel != "" {
		found := false
		for _, t := range c.Tunnels {
			if t.Name == c.Failover.PreferredTunnel {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("failover.preferred_tunnel %q does not match any tunnel", c.Failover.PreferredTunnel)
		}
	}
	if (c.Failover.Mode == FailoverModeManual || c.Failover.Mode == FailoverModePreferred) && c.Failover.PreferredTunnel == "" {
		return fmt.Errorf("failover.preferred_tunnel is required when mode is %q", c.Failover.Mode)
	}
	if c.Relay.Enabled {
		listen := c.Relay.EffectiveListen()
		host, portStr, err := splitHostPort(listen)
		if err != nil {
			return fmt.Errorf("relay.listen: %w", err)
		}
		if host == "" {
			return fmt.Errorf("relay.listen must include a host")
		}
		if portStr == 0 {
			return fmt.Errorf("relay.listen must include a port")
		}
		tag := c.Relay.EffectiveOutboundTag()
		if tag == c.Xray.Outbounds.Proxy.Tag || tag == c.Xray.Outbounds.Fallback.Tag || tag == "direct" || tag == "block" || tag == "api" {
			return fmt.Errorf("relay.outbound_tag %q conflicts with a built-in outbound", tag)
		}
		seenDomain := map[string]bool{}
		for _, s := range c.Relay.Sites {
			if s.Domain == "" {
				return fmt.Errorf("relay site has empty domain")
			}
			if seenDomain[s.Domain] {
				return fmt.Errorf("duplicate relay site %q", s.Domain)
			}
			seenDomain[s.Domain] = true
		}
	}
	return nil
}

// validate ensures all range-or-int strings are well-formed and integer
// counts are non-negative. Values are otherwise passed through to xray
// which performs its own bounds checking at startup.
func (t XHTTPInboundTuning) validate(prefix string) error {
	if err := validateRangeOrInt(prefix+".x_padding_bytes", t.XPaddingBytes); err != nil {
		return err
	}
	if t.ScMaxBufferedPosts < 0 {
		return fmt.Errorf("%s.sc_max_buffered_posts must be >= 0", prefix)
	}
	if err := validateRangeOrInt(prefix+".sc_max_each_post_bytes", t.ScMaxEachPostBytes); err != nil {
		return err
	}
	if err := validateRangeOrInt(prefix+".sc_stream_up_server_secs", t.ScStreamUpServerSecs); err != nil {
		return err
	}
	if t.KeepAlivePeriod < 0 {
		return fmt.Errorf("%s.keep_alive_period must be >= 0", prefix)
	}
	return nil
}

func (e XHTTPClientExtra) validate(prefix string) error {
	if err := validateRangeOrInt(prefix+".x_padding_bytes", e.XPaddingBytes); err != nil {
		return err
	}
	if e.Xmux != nil {
		if err := validateRangeOrInt(prefix+".xmux.max_concurrency", e.Xmux.MaxConcurrency); err != nil {
			return err
		}
		if err := validateRangeOrInt(prefix+".xmux.max_connections", e.Xmux.MaxConnections); err != nil {
			return err
		}
		if e.Xmux.CMaxReuseTimes < 0 {
			return fmt.Errorf("%s.xmux.c_max_reuse_times must be >= 0", prefix)
		}
		if e.Xmux.CMaxLifetimeMs < 0 {
			return fmt.Errorf("%s.xmux.c_max_lifetime_ms must be >= 0", prefix)
		}
		if e.Xmux.HMaxRequestTimes < 0 {
			return fmt.Errorf("%s.xmux.h_max_request_times must be >= 0", prefix)
		}
	}
	return nil
}

// validateRangeOrInt accepts an empty string (omit), a non-negative
// integer ("1000000"), or a non-negative range ("100-1000" with lo<=hi).
func validateRangeOrInt(field, v string) error {
	if v == "" {
		return nil
	}
	if lo, hi, ok := strings.Cut(v, "-"); ok {
		l, err := strconv.Atoi(lo)
		if err != nil || l < 0 {
			return fmt.Errorf("%s: invalid range %q", field, v)
		}
		h, err := strconv.Atoi(hi)
		if err != nil || h < l {
			return fmt.Errorf("%s: invalid range %q", field, v)
		}
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return fmt.Errorf("%s: not an int or range %q", field, v)
	}
	return nil
}

// ParseHHMM accepts a "HH:MM" 24-hour time string and returns the (hour, minute)
// pair. Used for daily-reset scheduling.
func ParseHHMM(s string) (hour, minute int, err error) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected HH:MM, got %q", s)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return 0, 0, fmt.Errorf("invalid hour in %q", s)
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("invalid minute in %q", s)
	}
	return h, m, nil
}

func splitHostPort(addr string) (string, int, error) {
	h, p, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, err
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port %q", p)
	}
	if n < 1 || n > 65535 {
		return "", 0, fmt.Errorf("port out of range: %d", n)
	}
	return h, n, nil
}
