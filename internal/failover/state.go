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
	Current           Mode   `json:"current"`
	Desired           Mode   `json:"desired"`
	Effective         Mode   `json:"effective"`
	Pending           bool   `json:"pending"`
	ConfirmationCount int    `json:"confirmation_count"`
	Reason            string `json:"reason"`
}

type State struct {
	Candidate      Mode  `json:"candidate"`
	Count          int   `json:"count"`
	LastChangeUnix int64 `json:"last_change_unix"`
}

func CurrentMode(cfg stack.Config) Mode {
	return Mode{OutboundTag: cfg.Xray.Outbounds.Proxy.Tag, Interface: cfg.Xray.Outbounds.Proxy.Interface}
}

func DesiredMode(cfg stack.Config, checks []tunnel.Check) Mode {
	for _, t := range cfg.Tunnels {
		for _, c := range checks {
			if c.Interface == t.Interface && c.Healthy {
				return Mode{OutboundTag: cfg.Xray.Outbounds.Proxy.Tag, Interface: t.Interface}
			}
		}
	}
	return Mode{OutboundTag: cfg.Failover.FallbackOutboundTag}
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
	if cfg.Failover.CooldownSeconds > 0 && state.LastChangeUnix > 0 && now.Unix()-state.LastChangeUnix < int64(cfg.Failover.CooldownSeconds) {
		decision.Pending = true
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
		cfg.Xray.Outbounds.Proxy.Interface = mode.Interface
	}
}
