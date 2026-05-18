# xray-stack-zeroone 🔢

Go control plane, edge relay, and web panel for the ZeroOne Xray stack.
The Python/Bash control plane was replaced by this Go service — see
[`docs/migration-cutover.md`](docs/migration-cutover.md).

## Layout

- `cmd/xray-stackd/` — Go control plane daemon. Renders Xray config, runs the
  admin API, supervises failover/VPN/relay, and serves the web panel.
- `cmd/edge-relay/` — small reverse proxy fronted by a managed PaaS
  (runflare, liara, …) so an Xray inbound can be exposed on a TLS-only
  platform. See [`docs/runflare-edge-deploy.md`](docs/runflare-edge-deploy.md).
- `internal/` — control-plane packages: `stack` (config), `xray` (config
  generation), `api` (HTTP), `auth`, `failover`, `relay`, `tunnel`,
  `bandwidth`, `enforce`, `usage`, `monitor`, `metrics`, `analytics`,
  `audit`, `events`, `notify`, `presence`, `sessions`, `snapshots`,
  `stats`, `subscription`, `firewall`, `links`, `system`.
- `web/app/` — React/Vite panel. Built into `web/app/dist/` and served by
  `xray-stackd` at `/`.
- `config/stack.example.json` — sanitized example config; copy to
  `config/stack.local.json` for local dev (gitignored).
- `config/examples/config.example.json` — sanitized example Xray config.
- `deploy/systemd/xray-stackd.service` — production unit file.
- `deploy/DEPLOY.md` — production install / upgrade notes.
- `rootfs/` — reduced server snapshot (nginx defaults, fallback site under
  `var/www/html`). Live xray configs, OpenVPN creds, and other sensitive
  files live here on disk for local reference but are gitignored.
- `scripts/` — `build.sh`, `check.sh`, `package.sh`,
  `install-local-layout.sh`, `sync-from-server.sh`, `import-live-stack.py`.

## Quick start

```bash
# Run tests + build a sample xray config
scripts/check.sh

# Build the production binary + panel into dist/
scripts/build.sh

# Package a release tarball for the server
scripts/package.sh
```

Run the daemon locally against a local config:

```bash
go run ./cmd/xray-stackd -config config/stack.local.json
```

Useful flags: `-print-xray` (render the generated Xray config and exit),
`-allow-apply` (let endpoints mutate live Xray/systemd state),
`-manage-failover`, `-manage-vpn`, `-manage-relay`.

## Sync from the live server

`scripts/sync-from-server.sh` rsyncs the live `/etc` and `/usr/local`
config into `rootfs/`. Run only when you intend to refresh the snapshot —
it overwrites local edits in `rootfs/`. Never rsync the result blindly
back into another machine.

## Security notes

- Live secrets (Xray configs, OpenVPN auth/keys, htpasswd, subscription
  user pages) sit under `rootfs/` for local reference and are all
  gitignored. Check `.gitignore` before adding anything under `rootfs/`.
- `config/stack.local.json` is gitignored; commit only
  `config/stack.example.json` changes.
