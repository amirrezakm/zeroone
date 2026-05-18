# Host install (no Docker)

This is the advanced install path: `xray-stackd` runs as a systemd
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
| `/usr/local/bin/xray-stackd` | The daemon binary. |
| `/usr/local/bin/xray` | Xray-core binary (download from [XTLS/Xray-core](https://github.com/XTLS/Xray-core/releases)). |
| `/usr/local/etc/xray-stack/stack.json` | Daemon config. |
| `/usr/local/etc/xray/config.json` | Live Xray config (rendered by the daemon). |
| `/usr/local/share/xray-stack-ui/` | React panel build. |
| `/var/lib/xray-stack/` | State: audit.log, snapshots, presence, failover state. |
| `/etc/systemd/system/xray-stackd.service` | Systemd unit. |
| `/etc/systemd/system/xray.service` | Xray service (sample in `deploy/skeleton/`). |
| `/etc/default/xray-stackd` | Env file consumed by the unit. |

## Build & install

```bash
git clone https://github.com/amirrezakm/zeroone.git
cd zeroone

# Build the daemon and the UI
scripts/build.sh
scripts/package.sh   # produces dist/zeroone-<sha>.tar.gz

# Install (run on the target host):
sudo install -m 0755 dist/xray-stackd /usr/local/bin/xray-stackd
sudo mkdir -p /usr/local/share/xray-stack-ui
sudo rsync -a --delete web/app/dist/ /usr/local/share/xray-stack-ui/
sudo mkdir -p /usr/local/etc/xray-stack /var/lib/xray-stack
sudo install -m 0600 config/stack.example.json /usr/local/etc/xray-stack/stack.json
sudo install -m 0644 deploy/systemd/xray-stackd.service /etc/systemd/system/
sudo install -m 0644 deploy/skeleton/xray.service       /etc/systemd/system/

# Install Xray-core separately from XTLS/Xray-core releases.
```

## Enable host-only features

Edit `/etc/default/xray-stackd`:

```ini
XRAY_STACKD_FLAGS=-allow-apply -manage-failover -manage-vpn -manage-relay
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

`xray-stackd -manage-vpn` watches the named systemd units and restarts
them if the interface goes down. `-manage-failover` flips the active
outbound based on `failover.probes`.

## Service management

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now xray-stackd.service
sudo journalctl -u xray-stackd -f
sudo systemctl restart xray-stackd.service
```

The unit is `Type=notify` with a 30s watchdog. The daemon notifies
systemd via `sd_notify(READY=1)` and pings the watchdog every 15s.

## Upgrading

Build the new tarball on a build host, copy it over, replace the binary
and UI files, and `systemctl restart`. The audit log and config are
preserved across upgrades.

```bash
sudo systemctl stop xray-stackd.service
sudo install -m 0755 dist/xray-stackd /usr/local/bin/xray-stackd
sudo rsync -a --delete web/app/dist/ /usr/local/share/xray-stack-ui/
sudo systemctl start xray-stackd.service
```

## When to switch to the Docker install

You almost always should — unless you specifically need the host-only
features above. The Docker image bundles a known-good Xray version,
keeps state inside one volume, and gives you a working installer.
