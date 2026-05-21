# CLAUDE.md

Guidance for Claude (or any AI assistant) working in this repo.

## What this repo is

`zeroone` — a Go control plane (`cmd/zeroone`) + edge relay
(`cmd/edge-relay`) + a React/Vite panel (`web/app`) for a single Xray
host. Distributed as a public open-source project under AGPL-3.0.

Module path: `github.com/amirrezakm/zeroone`. Public image:
`ghcr.io/amirrezakm/zeroone`.

## Build / test / run

- `go test ./...` — unit tests for all internal packages.
- `go build ./...` — compile every package; cheap sanity check.
- `scripts/check.sh` — runs `go test ./...`, renders an Xray config, and
  builds the panel if `web/app/node_modules` exists.
- `scripts/build.sh` — `GOOS=linux GOARCH=amd64` production binary into
  `dist/zeroone` + Vite build into `web/app/dist`.
- `scripts/package.sh` — builds and emits `dist/zeroone-<sha>.tar.gz`.
- `go run ./cmd/zeroone -config config/stack.example.json -print-xray`
  — render the generated Xray config and exit. Safe to run anywhere.

Two deployment models:

1. **Docker (default)** — single container, `network_mode: host`, single
   volume at `/var/lib/zeroone`. Driven by `docker/Dockerfile` and the
   one-line installer at `scripts/install.sh`. Xray runs as a child
   process under `-manage-xray`.
2. **Host install (advanced)** — systemd unit
   (`deploy/systemd/zeroone.service`, `Type=notify`, watchdog 30s).
   The daemon notifies via `internal/system/sdnotify`. Required for
   OpenVPN failover, bandwidth shaping (`nft`/`tc`), and any host-level
   integration. See `docs/HOST-INSTALL.md`.

## Conventions to follow

- **Go only**, no Python.
- **Iranian PaaS CDNs (runflare, arvan, …) reject raw WebSocket
  upgrades.** Use `xhttp` endpoints behind them. See
  `docs/runflare-edge-deploy.md`.
- The daemon is the source of truth for the live Xray config. Anything
  that hand-edits the running Xray config file on the server will be
  overwritten the next time the panel applies a change.
- Mutating endpoints are gated behind `-allow-apply`; tests and local
  dev should leave it off.
- Host-side features (`-manage-failover`, `-manage-vpn`, bandwidth
  shaping) must be opt-in behind flags so the container build can leave
  them disabled.
- State files live at `/var/lib/zeroone` for both container and host
  installs (`audit.log`, `snapshots/`, `presence.json`, failover state).
  The daemon creates them as needed and auto-migrates any pre-rebrand
  `/var/lib/xray-stack` directory on first run.
- Never commit anything under `config/stack.local.json`,
  `server-snapshots/`, or local secrets. The `.gitignore` guards these.

## What lives where

- `cmd/zeroone/` — daemon entrypoint.
- `cmd/edge-relay/` — small reverse proxy for PaaS edges (Runflare etc.).
  Provider-agnostic Dockerfile + a Runflare-specific
  `Dockerfile.runflare`.
- `internal/stack` — typed config (`stack.json`) + load/save + mutate.
- `internal/xray` — translates stack config into an Xray JSON config.
- `internal/api` — admin HTTP API; one file per concern (`admins.go`,
  `tokens.go`, `users_periods.go`, `notifications.go`,
  `observability.go`, `relay.go`).
- `internal/failover`, `internal/relay`, `internal/tunnel` — outbound
  health + supervision. `tunnel` is the lower-level VPN/route piece;
  `relay` supervises the mhrv-rs plugin; `failover` decides which
  outbound is live.
- `internal/auth` + `internal/sessions` + `internal/enforce` — admin
  session tokens, kill-session, and per-user quota enforcement.
- `internal/subscription` — `/sub/{token}` and `/me/{token}` user portal
  + link encoding.
- `internal/snapshots`, `internal/audit`, `internal/events`,
  `internal/notify`, `internal/presence`, `internal/metrics`,
  `internal/analytics`, `internal/stats` — observability and history.
- `internal/system` — small process/runner/sdnotify helpers.
- `internal/xrayinstall` — panel-driven Xray binary + geo data updates.
  Writes newer versions into a writable override tree at
  `/var/lib/zeroone/xray/` (`bin/xray` symlink, `versions/<ver>/`,
  `assets/{geoip,geosite}.dat`, `state.json`). When the override exists
  it takes precedence over the image-baked `/usr/local/bin/xray`;
  otherwise the daemon runs the image binary unchanged — no first-boot
  download. Mirror via `ZEROONE_XRAY_RELEASE_MIRROR` /
  `ZEROONE_XRAY_ASSETS_MIRROR` env vars or per-install in
  `stack.json:xray_update`.
- `docker/` — production Dockerfile, compose file, env example.
- `deploy/skeleton/` — sanitized reference systemd units for host
  installs.

## Public release

- License: AGPL-3.0-or-later (see `LICENSE`).
- Public release plan and implementation order live in the project plan
  file; current rollout sequence is in `CHANGELOG.md` under
  `[Unreleased]`.
- Never commit real-server IPs, FQDNs, UUIDs, or credentials. Example
  configs must use RFC-5737 TEST-NET ranges and `example.com`.
