package stack

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"slices"
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

func (c *Config) AddDirectDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("domain is required")
	}
	if !slices.Contains(c.Xray.Routing.DirectDomains, domain) {
		c.Xray.Routing.DirectDomains = append(c.Xray.Routing.DirectDomains, domain)
	}
	return nil
}

func (c *Config) DeleteDirectDomain(domain string) error {
	c.Xray.Routing.DirectDomains = slices.DeleteFunc(c.Xray.Routing.DirectDomains, func(v string) bool { return v == domain })
	return nil
}

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
