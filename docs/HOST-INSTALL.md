# Host install (no Docker)

This is the advanced install path: `zeroone` runs as a systemd
service alongside `xray.service` and (optionally) OpenVPN tunnel units.
Use this when you need:

- OpenVPN failover (the `-manage-vpn` and `-manage-failover` flags).
- Kernel-level per-user bandwidth shaping via `nft` + `tc`.
- Tight integration with an existing systemd-managed nginx / Xray setup.

Otherwise the Docker install (see [`INSTALL.md`](INSTALL.md)) is simpler
and the recommended path.

## Layout on the host

| Path | Purpose |
|---|---|
| `/usr/local/bin/zeroone` | The daemon binary. |
| `/usr/local/bin/xray` | Xray-core binary (download from [XTLS/Xray-core](https://github.com/XTLS/Xray-core/releases)). |
| `/usr/local/etc/zeroone/stack.json` | Daemon config. |
| `/usr/local/etc/xray/config.json` | Live Xray config (rendered by the daemon). |
| `/usr/local/share/zeroone-ui/` | React panel build. |
| `/var/lib/zeroone/` | State: audit.log, snapshots, presence, failover state. |
| `/etc/systemd/system/zeroone.service` | Systemd unit. |
| `/etc/systemd/system/xray.service` | Xray service (sample in `deploy/skeleton/`). |
| `/etc/default/zeroone` | Env file consumed by the unit. |

If you are upgrading from a pre-rebrand install, the daemon
auto-migrates `/var/lib/xray-stack` → `/var/lib/zeroone` on first start
when the new directory does not yet exist.

## Build & install

```bash
git clone https://github.com/amirrezakm/zeroone.git
cd zeroone

# Build the daemon and the UI
scripts/build.sh
scripts/package.sh   # produces dist/zeroone-<sha>.tar.gz

# Install (run on the target host):
sudo install -m 0755 dist/zeroone /usr/local/bin/zeroone
sudo mkdir -p /usr/local/share/zeroone-ui
sudo rsync -a --delete web/app/dist/ /usr/local/share/zeroone-ui/
sudo mkdir -p /usr/local/etc/zeroone /var/lib/zeroone
sudo install -m 0600 config/stack.example.json /usr/local/etc/zeroone/stack.json
sudo install -m 0644 deploy/systemd/zeroone.service /etc/systemd/system/
sudo install -m 0644 deploy/skeleton/xray.service   /etc/systemd/system/

# Install Xray-core separately from XTLS/Xray-core releases.
```

## Enable host-only features

Edit `/etc/default/zeroone`:

```ini
ZEROONE_FLAGS=-allow-apply -manage-failover -manage-vpn -manage-relay
```

The available flags:

- `-allow-apply` — let API endpoints mutate live Xray config and run
  `systemctl restart xray.service`. Required for any write operation.
- `-manage-failover` — run the automatic outbound failover loop that
  probes upstreams and rewrites the active outbound.
- `-manage-vpn` — restart tunnel systemd units (e.g.
  `openvpn@company.service`) when their interface goes down.
- `-manage-relay` — supervise the optional `mhrv-rs` relay plugin.
- `-manage-xray` — run Xray as a child process instead of via systemd.
  **Off** by default in host installs (systemd owns the process).

## Bandwidth shaping

The `internal/bandwidth` package writes nftables rules and `tc` qdiscs
per user. This requires:

- `nftables` package installed (`apt install nftables` /
  `dnf install nftables`).
- `iproute2` (always present on modern Linux).
- The daemon running as root or with `CAP_NET_ADMIN`.

Set `server.bandwidth_device` in `stack.json` to the public interface
(e.g. `eth0`).

## OpenVPN tunnels

Tunnel units are defined in `stack.json` under the `tunnels` array:

```json
"tunnels": [
  {
    "name": "primary",
    "type": "openvpn",
    "interface": "tun0",
    "systemd_unit": "openvpn@primary",
    "priority": 10
  }
]
```

`zeroone -manage-vpn` watches the named systemd units and restarts
them if the interface goes down. `-manage-failover` flips the active
outbound based on `failover.probes`.

## Service management

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now zeroone.service
sudo journalctl -u zeroone -f
sudo systemctl restart zeroone.service
```

The unit is `Type=notify` with a 30s watchdog. The daemon notifies
systemd via `sd_notify(READY=1)` and pings the watchdog every 15s.

## Upgrading

Build the new tarball on a build host, copy it over, replace the binary
and UI files, and `systemctl restart`. The audit log and config are
preserved across upgrades.

```bash
sudo systemctl stop zeroone.service
sudo install -m 0755 dist/zeroone /usr/local/bin/zeroone
sudo rsync -a --delete web/app/dist/ /usr/local/share/zeroone-ui/
sudo systemctl start zeroone.service
```

## When to switch to the Docker install

You almost always should — unless you specifically need the host-only
features above. The Docker image bundles a known-good Xray version,
keeps state inside one volume, and gives you a working installer.
