package stack

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"regexp"
	"slices"
	"strings"
)

func (c *Config) AddUser(email, uuid string) error {
	if email == "" {
		return fmt.Errorf("email is required")
	}
	if uuid == "" {
		uuid = newUUID()
	}
	for _, u := range c.Xray.Users {
		if u.Email == email {
			return fmt.Errorf("user %q already exists", email)
		}
	}
	c.Xray.Users = append(c.Xray.Users, User{Email: email, UUID: uuid, Enabled: true})
	return c.Validate()
}

func (c *Config) DeleteUser(email string) error {
	before := len(c.Xray.Users)
	c.Xray.Users = slices.DeleteFunc(c.Xray.Users, func(u User) bool { return u.Email == email })
	if len(c.Xray.Users) == before {
		return fmt.Errorf("user %q not found", email)
	}
	return c.Validate()
}

func (c *Config) SetUserEnabled(email string, enabled bool) error {
	for i := range c.Xray.Users {
		if c.Xray.Users[i].Email == email {
			c.Xray.Users[i].Enabled = enabled
			if enabled {
				c.Xray.Users[i].BannedUntil = 0
			}
			return c.Validate()
		}
	}
	return fmt.Errorf("user %q not found", email)
}

func (c *Config) UpdateUser(oldEmail, email, uuid string, enabled bool) error {
	if oldEmail == "" || email == "" || uuid == "" {
		return fmt.Errorf("old_email, email, and uuid are required")
	}
	for i := range c.Xray.Users {
		if c.Xray.Users[i].Email != oldEmail {
			continue
		}
		for j, other := range c.Xray.Users {
			if j != i && other.Email == email {
				return fmt.Errorf("user %q already exists", email)
			}
		}
		c.Xray.Users[i].Email = email
		c.Xray.Users[i].UUID = uuid
		c.Xray.Users[i].Enabled = enabled
		if enabled {
			c.Xray.Users[i].BannedUntil = 0
		}
		return c.Validate()
	}
	return fmt.Errorf("user %q not found", oldEmail)
}

func (c *Config) BanUser(email string, until int64) error {
	if until <= 0 {
		return fmt.Errorf("ban expiry is required")
	}
	for i := range c.Xray.Users {
		if c.Xray.Users[i].Email == email {
			c.Xray.Users[i].Enabled = false
			c.Xray.Users[i].BannedUntil = until
			return c.Validate()
		}
	}
	return fmt.Errorf("user %q not found", email)
}

func (c *Config) UnbanUser(email string) error {
	for i := range c.Xray.Users {
		if c.Xray.Users[i].Email == email {
			c.Xray.Users[i].Enabled = true
			c.Xray.Users[i].BannedUntil = 0
			return c.Validate()
		}
	}
	return fmt.Errorf("user %q not found", email)
}

func (c *Config) SetUserQuota(email string, quotaBytes int64) error {
	for i := range c.Xray.Users {
		if c.Xray.Users[i].Email == email {
			c.Xray.Users[i].QuotaBytes = quotaBytes
			return c.Validate()
		}
	}
	return fmt.Errorf("user %q not found", email)
}

// SetUserPeriodQuotas updates the daily/weekly/monthly byte caps and the
// daily reset time. Pass zero for any period to clear that cap.
// dailyResetHHMM may be empty (server-side default "00:00" applies).
func (c *Config) SetUserPeriodQuotas(email string, daily, weekly, monthly int64, dailyResetHHMM string) error {
	if daily < 0 || weekly < 0 || monthly < 0 {
		return fmt.Errorf("period quotas must be non-negative")
	}
	if dailyResetHHMM != "" {
		if _, _, err := ParseHHMM(dailyResetHHMM); err != nil {
			return err
		}
	}
	for i := range c.Xray.Users {
		if c.Xray.Users[i].Email == email {
			c.Xray.Users[i].DailyQuotaBytes = daily
			c.Xray.Users[i].WeeklyQuotaBytes = weekly
			c.Xray.Users[i].MonthlyQuotaBytes = monthly
			c.Xray.Users[i].DailyResetHHMM = dailyResetHHMM
			return c.Validate()
		}
	}
	return fmt.Errorf("user %q not found", email)
}

// SetUserMaxSessions updates the concurrent-device cap. Zero clears it.
func (c *Config) SetUserMaxSessions(email string, maxSessions int) error {
	if maxSessions < 0 {
		return fmt.Errorf("max_sessions must be >= 0")
	}
	for i := range c.Xray.Users {
		if c.Xray.Users[i].Email == email {
			c.Xray.Users[i].MaxSessions = maxSessions
			return c.Validate()
		}
	}
	return fmt.Errorf("user %q not found", email)
}

// AddAdmin appends a new panel administrator. The hash must already be
// pbkdf2-sha256 formatted; callers should use auth.HashPassword.
func (c *Config) AddAdmin(username, passwordHash string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("username is required")
	}
	if passwordHash == "" {
		return fmt.Errorf("password_hash is required")
	}
	for _, a := range c.Panel.Admins {
		if a.Username == username {
			return fmt.Errorf("admin %q already exists", username)
		}
	}
	c.Panel.Admins = append(c.Panel.Admins, Admin{
		Username:     username,
		PasswordHash: passwordHash,
	})
	return c.Validate()
}

// SetAdminPassword updates the password hash for an existing admin.
func (c *Config) SetAdminPassword(username, passwordHash string) error {
	if passwordHash == "" {
		return fmt.Errorf("password_hash is required")
	}
	for i := range c.Panel.Admins {
		if c.Panel.Admins[i].Username == username {
			c.Panel.Admins[i].PasswordHash = passwordHash
			return c.Validate()
		}
	}
	return fmt.Errorf("admin %q not found", username)
}

// DeleteAdmin removes an admin by username. Refuses to delete the last
// remaining admin so the panel doesn't lock itself out.
func (c *Config) DeleteAdmin(username string) error {
	if len(c.Panel.Admins) <= 1 {
		return fmt.Errorf("cannot delete the last remaining admin")
	}
	before := len(c.Panel.Admins)
	c.Panel.Admins = slices.DeleteFunc(c.Panel.Admins, func(a Admin) bool { return a.Username == username })
	if len(c.Panel.Admins) == before {
		return fmt.Errorf("admin %q not found", username)
	}
	return c.Validate()
}

func (c *Config) SetUserBandwidth(email string, downloadMbps, uploadMbps int) error {
	if downloadMbps < 0 || uploadMbps < 0 {
		return fmt.Errorf("bandwidth must be non-negative")
	}
	for i := range c.Xray.Users {
		if c.Xray.Users[i].Email == email {
			c.Xray.Users[i].DownloadMbps = downloadMbps
			c.Xray.Users[i].UploadMbps = uploadMbps
			if downloadMbps == 0 && uploadMbps == 0 {
				c.Xray.Users[i].BandwidthPort = 0
			} else if c.Xray.Users[i].BandwidthPort == 0 {
				port, err := c.NextBandwidthPort()
				if err != nil {
					return err
				}
				c.Xray.Users[i].BandwidthPort = port
			}
			return c.Validate()
		}
	}
	return fmt.Errorf("user %q not found", email)
}

func (c Config) NextBandwidthPort() (int, error) {
	used := map[int]bool{}
	add := func(port int) {
		if port > 0 {
			used[port] = true
		}
	}
	add(c.Xray.Inbounds.VLESSWSPort)
	add(c.Xray.Inbounds.VLESSXHTTPPort)
	add(c.Xray.Inbounds.LocalSOCKSPort)
	add(c.Xray.APIPort)
	for _, s := range c.Xray.Inbounds.PublicSOCKS {
		add(s.Port)
	}
	for _, u := range c.Xray.Users {
		add(u.BandwidthPort)
	}
	for port := 21000; port < 22000; port++ {
		if !used[port] {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free bandwidth port available")
}

func (c *Config) UpsertClientEndpoint(ep ClientEndpoint) error {
	ep.Name = strings.TrimSpace(ep.Name)
	ep.Host = strings.TrimSpace(strings.ToLower(ep.Host))
	ep.Network = strings.TrimSpace(strings.ToLower(ep.Network))
	ep.Path = strings.TrimSpace(ep.Path)
	ep.Mode = strings.TrimSpace(strings.ToLower(ep.Mode))
	if ep.Name == "" {
		return fmt.Errorf("name is required")
	}
	if ep.Host == "" {
		return fmt.Errorf("host is required")
	}
	if ep.Port == 0 {
		if ep.TLS {
			ep.Port = 443
		} else {
			ep.Port = 80
		}
	}
	if ep.Network == "" {
		ep.Network = "ws"
	}
	if ep.Path == "" {
		if ep.Network == "xhttp" {
			ep.Path = "/xhttp"
		} else {
			ep.Path = "/vless"
		}
	}
	if ep.Network == "xhttp" && ep.Mode == "" {
		ep.Mode = c.Xray.Inbounds.EffectiveVLESSXHTTPMode()
	}
	found := false
	for i := range c.Server.ClientEndpoints {
		if c.Server.ClientEndpoints[i].Name == ep.Name {
			c.Server.ClientEndpoints[i] = ep
			found = true
			break
		}
	}
	if !found {
		c.Server.ClientEndpoints = append(c.Server.ClientEndpoints, ep)
	}
	return c.Validate()
}

func (c *Config) DeleteClientEndpoint(name string) error {
	name = strings.TrimSpace(name)
	before := len(c.Server.ClientEndpoints)
	c.Server.ClientEndpoints = slices.DeleteFunc(c.Server.ClientEndpoints, func(ep ClientEndpoint) bool { return ep.Name == name })
	if len(c.Server.ClientEndpoints) == before {
		return fmt.Errorf("client endpoint %q not found", name)
	}
	return c.Validate()
}

func (c *Config) AddDirectDomain(domain string) error {
	cleaned, err := NormalizeDomainRule(domain)
	if err != nil {
		return err
	}
	if !slices.Contains(c.Xray.Routing.DirectDomains, cleaned) {
		c.Xray.Routing.DirectDomains = append(c.Xray.Routing.DirectDomains, cleaned)
	}
	return nil
}

func (c *Config) DeleteDirectDomain(domain string) error {
	c.Xray.Routing.DirectDomains = slices.DeleteFunc(c.Xray.Routing.DirectDomains, func(v string) bool { return v == domain })
	return nil
}

// NormalizeDomainRule cleans up user-supplied domain input. It accepts:
//   - URLs ("https://example.com/path") — scheme and path are stripped
//   - bare hostnames ("example.com") — kept as substring match (no prefix)
//   - prefixed forms ("domain:foo", "full:foo", "regexp:..."), kept verbatim
//
// The hostname portion is lowercased and validated against a permissive
// hostname pattern; trailing slashes, ports, and surrounding whitespace are
// removed.
func NormalizeDomainRule(input string) (string, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", fmt.Errorf("domain is required")
	}
	// Preserve user-supplied prefix forms — they know what they're asking for.
	for _, p := range []string{"domain:", "full:", "regexp:", "geosite:", "ext:"} {
		if strings.HasPrefix(s, p) {
			rest := strings.TrimSpace(s[len(p):])
			if rest == "" {
				return "", fmt.Errorf("%s prefix needs a value", p)
			}
			return p + rest, nil
		}
	}
	// Strip URL scheme/path/query so "https://geroogo.com/" → "geroogo.com".
	if idx := strings.Index(s, "://"); idx >= 0 {
		s = s[idx+3:]
	}
	if idx := strings.IndexAny(s, "/?#"); idx >= 0 {
		s = s[:idx]
	}
	// Drop a trailing port if present.
	if h, _, err := net.SplitHostPort(s); err == nil {
		s = h
	}
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return "", fmt.Errorf("domain is required")
	}
	if !validHostname.MatchString(s) {
		return "", fmt.Errorf("not a valid hostname: %q", input)
	}
	return s, nil
}

var validHostname = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

func (c *Config) AddSOCKS(s SOCKSInbound) error {
	if s.Name == "" || s.Username == "" {
		return fmt.Errorf("name and username are required")
	}
	if s.Password == "" {
		s.Password = randomToken(9)
	}
	for _, existing := range c.Xray.Inbounds.PublicSOCKS {
		if existing.Name == s.Name || existing.Port == s.Port || existing.Username == s.Username {
			return fmt.Errorf("duplicate SOCKS name, username, or port")
		}
	}
	c.Xray.Inbounds.PublicSOCKS = append(c.Xray.Inbounds.PublicSOCKS, s)
	return c.Validate()
}

func (c *Config) UpdateSOCKS(oldUsername string, next SOCKSInbound) error {
	if oldUsername == "" || next.Name == "" || next.Username == "" || next.Port <= 0 {
		return fmt.Errorf("old_username, name, username, and port are required")
	}
	for i := range c.Xray.Inbounds.PublicSOCKS {
		if c.Xray.Inbounds.PublicSOCKS[i].Username != oldUsername {
			continue
		}
		if next.Password == "" {
			next.Password = c.Xray.Inbounds.PublicSOCKS[i].Password
		}
		for j, existing := range c.Xray.Inbounds.PublicSOCKS {
			if j == i {
				continue
			}
			if existing.Name == next.Name || existing.Port == next.Port || existing.Username == next.Username {
				return fmt.Errorf("duplicate SOCKS name, username, or port")
			}
		}
		c.Xray.Inbounds.PublicSOCKS[i] = next
		return c.Validate()
	}
	return fmt.Errorf("SOCKS user %q not found", oldUsername)
}

// SetFailoverMode updates the failover mode and (when not auto) the preferred
// tunnel. Mode is normalized to lowercase; empty means "auto". For manual or
// preferred modes, preferredTunnel must match an existing tunnel name.
func (c *Config) SetFailoverMode(mode, preferredTunnel string) error {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		mode = FailoverModeAuto
	}
	preferredTunnel = strings.TrimSpace(preferredTunnel)
	switch mode {
	case FailoverModeAuto:
		c.Failover.Mode = FailoverModeAuto
		c.Failover.PreferredTunnel = ""
	case FailoverModeManual, FailoverModePreferred:
		if preferredTunnel == "" {
			return fmt.Errorf("preferred_tunnel is required for mode %q", mode)
		}
		found := false
		for _, t := range c.Tunnels {
			if t.Name == preferredTunnel {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("preferred_tunnel %q does not match any tunnel", preferredTunnel)
		}
		c.Failover.Mode = mode
		c.Failover.PreferredTunnel = preferredTunnel
	default:
		return fmt.Errorf("mode must be auto, manual, or preferred")
	}
	return c.Validate()
}

func (c *Config) DeleteSOCKS(username string) error {
	before := len(c.Xray.Inbounds.PublicSOCKS)
	c.Xray.Inbounds.PublicSOCKS = slices.DeleteFunc(c.Xray.Inbounds.PublicSOCKS, func(s SOCKSInbound) bool { return s.Username == username })
	if len(c.Xray.Inbounds.PublicSOCKS) == before {
		return fmt.Errorf("SOCKS user %q not found", username)
	}
	return c.Validate()
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}
