package stack

// EnsureSubTokens generates a SubToken for any enabled user that doesn't
// have one yet. Returns true if at least one token was filled in so the
// caller knows to persist the config. Safe to call on every startup —
// no-op when all users already have tokens.
func (c *Config) EnsureSubTokens() bool {
	changed := false
	for i := range c.Xray.Users {
		if c.Xray.Users[i].SubToken == "" {
			c.Xray.Users[i].SubToken = randomToken(32)
			changed = true
		}
	}
	return changed
}

// UserBySubToken returns the user (and true) matching the given token, or
// a zero user (and false) if no user has that token. Empty input always
// returns false so a missing token can't accidentally authenticate.
func (c Config) UserBySubToken(token string) (User, bool) {
	if token == "" {
		return User{}, false
	}
	for _, u := range c.Xray.Users {
		if u.SubToken == token {
			return u, true
		}
	}
	return User{}, false
}
