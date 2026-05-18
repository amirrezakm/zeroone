# Go Rewrite Plan

## Goal

Replace the patched Python/Bash control plane with a maintainable Go service while keeping Xray, OpenVPN, nginx, and systemd as runtime components.

## Target Components

- `xray-stackd`: one Go daemon for admin API, config generation, health checks, failover, usage accounting, and controlled apply/restart.
- `web/app`: separate UI project. It talks to the daemon API and can later be served by nginx or embedded into the daemon.
- `config/stack.local.json`: local-only source-of-truth imported from the current server snapshot. This file contains secrets and is ignored by git.
- Generated runtime config: `/usr/local/etc/xray/config.json`.

## Migration Strategy

1. Keep production unchanged while developing locally.
2. Generate `stack.local.json` from the current live Xray config using `scripts/import-live-stack.py`.
3. Generate Xray config from Go and compare it against the current live config behavior.
4. Add read-only status API and UI.
5. Add write flows one by one: user management, SOCKS management, routing, failover, usage reset.
6. Deploy the Go daemon beside the old monitor first on a different local port.
7. Verify dashboard, generated config, failover decisions, and usage accounting.
8. Cut over within the allowed downtime window by switching systemd/nginx to the new daemon and disabling old Python/Bash services.

## Design Decisions

- Xray remains the data plane. Go owns the source-of-truth and generates Xray config.
- Tunnel providers are adapter-based. OpenVPN is the first adapter; WireGuard or other providers can be added behind the same interface.
- Failover is a state machine with confirmations, cooldown, and explicit reason logging.
- Usage reset means `baseline=current` and `total=0`, so active users do not resurrect old traffic.
- Every config apply must create a backup, run `xray run -test`, then restart Xray only after validation.
