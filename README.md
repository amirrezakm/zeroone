# zeroone

> A Go-based control panel for [Xray-core](https://github.com/XTLS/Xray-core),
> with subscription links, traffic analytics, and a one-line installer.

[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](LICENSE)

`zeroone` is a single-host Xray control plane:

- **Go daemon** that renders Xray config, supervises the `xray` process, and
  exposes a REST API.
- **React panel** for user management, subscription links, routing rules,
  traffic analytics, and snapshots.
- **Docker-first** deployment — `network_mode: host`, single volume,
  one container.
- **One-line installer** that drops a `zeroone` CLI at
  `/usr/local/bin/zeroone` and gets the panel running in under 90 seconds.

## 🚀 Install

### Standard install (one line)

On a clean Ubuntu / Debian / RHEL / Alma / Rocky / Alpine VPS with
internet access:

```bash
sudo bash -c "$(curl -sSL https://raw.githubusercontent.com/amirrezakm/zeroone/main/scripts/install.sh)" @ install
```

The installer will:

1. Install Docker if missing.
2. Prompt for an admin username + password.
3. Pull `ghcr.io/amirrezakm/zeroone:latest` and start the container.
4. Print the panel URL.

After install:

```bash
zeroone status        # service + healthcheck
zeroone logs -f       # follow daemon logs
zeroone update        # pull a newer image and restart
zeroone cli admin list
zeroone backup -o /root/zeroone-backup.tgz
```

See [`docs/INSTALL.md`](docs/INSTALL.md) for the full install guide and
[`docs/CLI.md`](docs/CLI.md) for every subcommand.

### 🇮🇷 Offline install (for Iranian servers / no GHCR access)

**If your server is inside Iran or otherwise can't reach
`ghcr.io`, the one-line installer above will fail** — the image pull
goes through GitHub's registry which is blocked from many Iranian
networks. Use the **offline install** instead.

Every release ships a **prebuilt offline bundle** as a GitHub asset.
You download it on any machine with internet, SFTP it to your
server, and run the installer locally — no registry access needed
on the destination.

**Three steps, ~3 minutes:**

```bash
# 1. Download from https://github.com/amirrezakm/zeroone/releases/latest
#    (pick the file matching your server's arch — amd64 or arm64)

# 2. Upload to the server
sftp root@YOUR_SERVER_IP
sftp> put zeroone-offline-*.tar.gz

# 3. SSH in and install
tar -xzf zeroone-offline-*.tar.gz
sudo bash install-offline.sh
```

The bundled `install-offline.sh` will even install Docker for you
via the Runflare Linux mirror if it's missing on the destination —
no manual setup required. Full guide:
**[`docs/OFFLINE-INSTALL.md`](docs/OFFLINE-INSTALL.md)** 📦

### Manual / Docker Compose install

```bash
mkdir -p /opt/zeroone /var/lib/zeroone
curl -sSL https://raw.githubusercontent.com/amirrezakm/zeroone/main/docker/docker-compose.yml > /opt/zeroone/docker-compose.yml
curl -sSL https://raw.githubusercontent.com/amirrezakm/zeroone/main/docker/.env.example       > /opt/zeroone/.env
cd /opt/zeroone && docker compose up -d
```

### Host install (no Docker)

For advanced operators who want OpenVPN failover, kernel-level bandwidth
shaping (`nft` / `tc`), or systemd-managed tunnels, see
[`docs/HOST-INSTALL.md`](docs/HOST-INSTALL.md).

## Features

- VLESS over WebSocket and XHTTP (works behind PaaS CDNs that reject raw
  WebSocket upgrades).
- Per-user UUIDs, quotas, daily reset windows, enable/disable.
- Subscription links at `/sub/{token}`, user portal at `/me/{token}`.
- Live traffic counters scraped from Xray's stats API.
- Routing rules: direct domains, blocked IPs/domains, AI-domain routing.
- Audit log + JSON snapshots of every config change.
- Multiple admin accounts with PBKDF2 password hashes.
- Healthcheck endpoint, `slog`-structured logs, OCI image labels.

## Repository layout

- `cmd/zeroone/` — control plane daemon.
- `cmd/edge-relay/` — small reverse proxy for PaaS edges. See
  [`docs/runflare-edge-deploy.md`](docs/runflare-edge-deploy.md).
- `internal/` — 24 Go packages: `stack` (config), `xray` (config
  generation), `api` (HTTP), `auth`, `failover`, `relay`, `tunnel`,
  `bandwidth`, `enforce`, `usage`, `monitor`, `metrics`, `analytics`,
  `audit`, `events`, `notify`, `presence`, `sessions`, `snapshots`,
  `stats`, `subscription`, `firewall`, `links`, `system`.
- `web/app/` — React/Vite panel.
- `docker/` — Dockerfile, compose file, env example.
- `scripts/` — `install.sh` (the one-line installer + CLI wrapper),
  `offline-bundle.sh` + `install-offline.sh` (air-gapped flow),
  `build.sh`, `check.sh`, `package.sh`.

## Development

```bash
# Run tests + render a sample xray config
scripts/check.sh

# Build the production binary + panel into dist/
scripts/build.sh

# Run the daemon locally
go run ./cmd/zeroone -config config/stack.example.json
```

Flags: `-print-xray` (render generated Xray config and exit),
`-allow-apply` (mutate live Xray/systemd state), `-manage-failover`,
`-manage-vpn`, `-manage-relay`, `-manage-xray` (run Xray as a child
process instead of via systemd).

## Documentation

- [`docs/INSTALL.md`](docs/INSTALL.md) — installer + manual install.
- [`docs/CLI.md`](docs/CLI.md) — `zeroone` subcommands.
- [`docs/CONFIG.md`](docs/CONFIG.md) — annotated `stack.json` reference.
- [`docs/UPGRADE.md`](docs/UPGRADE.md) — upgrade flow.
- [`docs/SECURITY.md`](docs/SECURITY.md) — TLS, reverse proxy, port hardening.
- [`docs/HOST-INSTALL.md`](docs/HOST-INSTALL.md) — systemd install with
  OpenVPN / failover / bandwidth shaping.
- [`docs/OFFLINE-INSTALL.md`](docs/OFFLINE-INSTALL.md) — air-gapped
  install via SFTP + `docker save`/`docker load` (Iranian operators).
- [`docs/runflare-edge-deploy.md`](docs/runflare-edge-deploy.md) — edge
  relay deployment on PaaS providers.

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md). All changes go through pull
requests against `main`.

## Security

Please report security issues privately — see [`SECURITY.md`](SECURITY.md).

## License

[AGPL-3.0-or-later](LICENSE). Fork-friendly, but networked deployments must
make their modifications available under the same license.
