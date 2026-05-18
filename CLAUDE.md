# CLAUDE.md

Guidance for Claude (or any AI assistant) working in this repo.

## What this repo is

Go control plane (`cmd/xray-stackd`) + edge relay (`cmd/edge-relay`) + a
React/Vite panel (`web/app`) for a single Xray production host
(`185.128.139.68`). The Python/Bash control plane was retired on
2026-05-10; see `docs/migration-cutover.md`.

## Build / test / run

- `go test ./...` — unit tests for all internal packages.
- `go build ./...` — compile every package; cheap sanity check.
- `scripts/check.sh` — regenerates `config/stack.local.json` (uses
  `import-live-stack.py`), runs `go test ./...`, renders an Xray config,
  and builds the panel if `web/app/node_modules` exists.
- `scripts/build.sh` — `GOOS=linux GOARCH=amd64` production binary into
  `dist/xray-stackd` + Vite build into `web/app/dist`.
- `scripts/package.sh` — builds and emits `dist/xray-stack-zeroone-<sha>.tar.gz`.
- `go run ./cmd/xray-stackd -config config/stack.local.json -print-xray` —
  render the generated Xray config and exit. Safe to run anywhere.

The production binary runs under systemd
(`deploy/systemd/xray-stackd.service`, `Type=notify`, watchdog 30s). The
daemon notifies via `internal/system/sdnotify`.

## Conventions to follow

- **Go only**, no Python additions. The one existing Python helper
  (`scripts/import-live-stack.py`) is a dev-only snapshot importer; do
  not extend it. If new tooling is needed, write Go or
  shell+jq/awk/openssl.
- **Iranian PaaS CDNs (runflare, arvan, …) reject raw WebSocket
  upgrades.** Use `xhttp` endpoints behind them. See
  `docs/runflare-edge-deploy.md`. The `client_endpoints` block in
  `config/stack.example.json` shows the pattern.
- The daemon is the source of truth for the live Xray config. Anything
  that hand-edits `/usr/local/etc/xray/config.json` on the server will be
  overwritten the next time the panel applies a change.
- Mutating endpoints are gated behind `-allow-apply`; tests and local
  dev should leave it off.
- State files live under `/var/lib/xray-stack` on the server
  (`audit.log`, `snapshots/`, `presence.json`, failover state). The
  daemon creates them as needed.

## What lives where

- `internal/stack` — typed config (`stack.json`) + load/save + mutate.
- `internal/xray` — translates stack config into an Xray JSON config.
- `internal/api` — admin HTTP API; one file per concern (`admins.go`,
  `tokens.go`, `users_periods.go`, `notifications.go`, `observability.go`,
  `relay.go`).
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

## Things to be careful with

- Never commit anything under `rootfs/usr/local/etc/xray/config*.json`,
  `rootfs/etc/openvpn/*`, `rootfs/etc/nginx/.monitor.htpasswd`,
  `config/stack.local.json`, or `server-snapshots/`. The `.gitignore`
  guards these but check before `git add -A`.
- Don't commit build artifacts: `dist/`, `edge-relay`, `xray-stackd`,
  `web/app/dist`, `web/app/node_modules`, `*.tar.gz`.
- The production server uses systemd `Type=notify` — keep the sdnotify
  calls in `cmd/xray-stackd/main.go` working or the service will be
  killed by the watchdog.
