package failover

import (
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
	"github.com/sakhtar/xray-stack-zeroone/internal/tunnel"
)

type Mode struct {
	OutboundTag string `json:"outbound_tag"`
	Interface   string `json:"interface,omitempty"`
}

type Decision struct {
	Current                  Mode   `json:"current"`
	Desired                  Mode   `json:"desired"`
	Effective                Mode   `json:"effective"`
	Pending                  bool   `json:"pending"`
	ConfirmationCount        int    `json:"confirmation_count"`
	CooldownRemainingSeconds int64  `json:"cooldown_remaining_seconds,omitempty"`
	Reason                   string `json:"reason"`
}

type State struct {
	Candidate      Mode  `json:"candidate"`
	Count          int   `json:"count"`
	LastChangeUnix int64 `json:"last_change_unix"`
}

func CurrentMode(cfg stack.Config) Mode {
	tag := cfg.Xray.Routing.AIOutboundTag
	if tag == "" {
		tag = cfg.Xray.Outbounds.Proxy.Tag
	}
	mode := Mode{OutboundTag: tag}
	if tag == cfg.Xray.Outbounds.Proxy.Tag {
		mode.Interface = cfg.Xray.Outbounds.Proxy.Interface
	}
	return mode
}

// tunnelHealthy reports whether the named tunnel (by config name) is healthy.
// Returns (healthy, interfaceName, found).
func tunnelHealthy(cfg stack.Config, checks []tunnel.Check, name string) (bool, string, bool) {
	for _, t := range cfg.Tunnels {
		if t.Name != name {
			continue
		}
		for _, c := range checks {
			if c.Interface == t.Interface {
				return c.Healthy, t.Interface, true
			}
		}
		return false, t.Interface, true
	}
	return false, "", false
}

// interfaceHealthy reports whether the given interface is in the check list as healthy.
func interfaceHealthy(checks []tunnel.Check, iface string) bool {
	if iface == "" {
		return false
	}
	for _, c := range checks {
		if c.Interface == iface {
			return c.Healthy
		}
	}
	return false
}

// firstHealthyMode walks tunnels in priority order, returning the proxy mode
// for the first healthy one. Falls back to the configured fallback outbound
// tag when nothing is healthy.
func firstHealthyMode(cfg stack.Config, checks []tunnel.Check) Mode {
	for _, t := range cfg.Tunnels {
		for _, c := range checks {
			if c.Interface == t.Interface && c.Healthy {
				return Mode{OutboundTag: cfg.Xray.Outbounds.Proxy.Tag, Interface: t.Interface}
			}
		}
	}
	return Mode{OutboundTag: cfg.Failover.FallbackOutboundTag}
}

// DesiredMode decides which proxy mode the failover loop wants to be in.
// Honors cfg.Failover.Mode: auto walks priority order, preferred biases toward
// PreferredTunnel when healthy, manual sticks with the current interface as
// long as it's healthy (drifts only on failure, doesn't auto-return).
func DesiredMode(cfg stack.Config, checks []tunnel.Check) Mode {
	mode := cfg.Failover.EffectiveMode()
	switch mode {
	case stack.FailoverModePreferred:
		if cfg.Failover.PreferredTunnel != "" {
			if healthy, iface, ok := tunnelHealthy(cfg, checks, cfg.Failover.PreferredTunnel); ok && healthy {
				return Mode{OutboundTag: cfg.Xray.Outbounds.Proxy.Tag, Interface: iface}
			}
		}
		return firstHealthyMode(cfg, checks)
	case stack.FailoverModeManual:
		current := CurrentMode(cfg)
		if current.Interface != "" && interfaceHealthy(checks, current.Interface) {
			return current
		}
		return firstHealthyMode(cfg, checks)
	default:
		return firstHealthyMode(cfg, checks)
	}
}

func Decide(cfg stack.Config, state State, checks []tunnel.Check, now time.Time) (Decision, State) {
	current := CurrentMode(cfg)
	desired := DesiredMode(cfg, checks)
	decision := Decision{Current: current, Desired: desired, Effective: current}
	if desired == current {
		state.Candidate = Mode{}
		state.Count = 0
		decision.Effective = desired
		decision.Reason = "already in desired mode"
		return decision, state
	}
	// Bypass cooldown when returning to the user's preferred tunnel: cooldown
	// exists to debounce flapping between equally-good tunnels, but the user
	// has explicitly told us "always prefer X." Without this, auto-return can
	// take up to confirmations*interval + cooldown (~6min on default config),
	// which feels broken.
	skipCooldown := false
	if cfg.Failover.EffectiveMode() == stack.FailoverModePreferred && cfg.Failover.PreferredTunnel != "" {
		for _, t := range cfg.Tunnels {
			if t.Name == cfg.Failover.PreferredTunnel && t.Interface == desired.Interface {
				skipCooldown = true
				break
			}
		}
	}
	if !skipCooldown && cfg.Failover.CooldownSeconds > 0 && state.LastChangeUnix > 0 && now.Unix()-state.LastChangeUnix < int64(cfg.Failover.CooldownSeconds) {
		decision.Pending = true
		decision.CooldownRemainingSeconds = int64(cfg.Failover.CooldownSeconds) - (now.Unix() - state.LastChangeUnix)
		decision.Reason = "cooldown active"
		return decision, state
	}
	if state.Candidate == desired {
		state.Count++
	} else {
		state.Candidate = desired
		state.Count = 1
	}
	decision.ConfirmationCount = state.Count
	confirmations := cfg.Failover.Confirmations
	if confirmations <= 0 {
		confirmations = 1
	}
	if state.Count < confirmations {
		decision.Pending = true
		decision.Reason = "waiting for confirmations"
		return decision, state
	}
	state.Candidate = Mode{}
	state.Count = 0
	state.LastChangeUnix = now.Unix()
	decision.Effective = desired
	decision.Reason = "confirmed change"
	return decision, state
}

func ApplyMode(cfg *stack.Config, mode Mode) {
	if mode.OutboundTag == cfg.Xray.Outbounds.Proxy.Tag {
		cfg.Xray.Routing.AIOutboundTag = ""
		cfg.Xray.Outbounds.Proxy.Interface = mode.Interface
		return
	}
	cfg.Xray.Routing.AIOutboundTag = mode.OutboundTag
}
