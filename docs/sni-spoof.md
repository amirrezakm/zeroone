# SNI-spoof plugin

The SNI-spoof plugin gives zeroone a built-in DPI-desync layer that xray
rides on top of. It is the supervised, Linux-native analogue of client-side
tools like patterniha/SNI-Spoofing (which is Windows-only): instead of a
WinDivert packet injector, it composes two well-known Linux pieces.

## How it works

```
xray (sni-spoof outbound, SO_MARK=fwmark)
   │  marked packets
   ▼
ip rule: fwmark <mark> → table <table> → default dev <tun>
   ▼
tun device  ──►  tun2socks  ──►  byedpi (SOCKS5, desync)  ──►  real destination
```

- **byedpi** (`/usr/local/bin/byedpi`) is a local SOCKS5 proxy that applies
  the desync. For the default `fake` method it injects a fake ClientHello
  carrying a **decoy SNI** with a low TTL, so the DPI sees an allowed name
  while the real server only ever receives the genuine ClientHello.
- **tun2socks** (`/usr/local/bin/tun2socks`) exposes a real tun device and
  forwards everything it receives to byedpi's SOCKS5 listener.
- A **scoped policy route** (fwmark → table → `default dev <tun>`) steers only
  xray's marked traffic into the tun. byedpi's own egress is unmarked, follows
  the normal routing table, and therefore never loops back through the tun.
- **xray on top**: the `sni-spoof` outbound is a `freedom` outbound that stamps
  `sockopt.mark`. Configured domains route to it; everything else is untouched.

The real SNI is preserved end-to-end — Cloudflare/your origin still routes by
the genuine name — and only the DPI is shown a decoy.

## Enabling

Add a `sni_spoof` block to `stack.json` (or use the panel / `PUT
/api/snispoof/config`) and start the daemon with `-manage-snispoof`:

```json
"sni_spoof": {
  "enabled": true,
  "fake_domain": "www.cloudflare.com",
  "method": "fake",
  "fake_ttl": 8,
  "listen": "127.0.0.1:8087",
  "outbound_tag": "sni-spoof",
  "tun_name": "znspoof0",
  "tun_addr": "10.99.0.1/24",
  "firewall_mark": 7137,
  "route_table": 7137,
  "mtu": 1420,
  "sites": [ { "domain": "domain:youtube.com", "enabled": true } ]
}
```

`method` is one of `fake`, `split`, `disorder`, `auto`. `strategy` (raw byedpi
flags) and `extra_args` override the preset for tuning against a specific DPI.

## Requirements

- `byedpi` and `tun2socks` binaries on the host (bundled in the container
  image; for host installs drop them in `/usr/local/bin`).
- `CAP_NET_ADMIN` for the daemon (sets `SO_MARK`, programs `ip rule`/route,
  creates the tun). Host installs run as root; the container needs the
  capability.
- xray needs `CAP_NET_ADMIN` too, since the `sni-spoof` outbound sets `SO_MARK`.

## Endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/snispoof/config` | current config + defaults |
| PUT | `/api/snispoof/config` | partial update (reloads supervisor) |
| POST/PUT/DELETE | `/api/snispoof/sites` | manage routed domains |
| GET | `/api/snispoof/status` | byedpi/tun2socks/route + probe health |
| POST | `/api/snispoof/test` | SOCKS5 CONNECT probe through byedpi |
| POST | `/api/snispoof/restart` | full teardown + respawn |
| GET | `/api/snispoof/logs` | tail the plugin log + supervisor events |

## Caveats

- Whether a given desync method actually defeats a particular DPI is
  network-specific — verify with `POST /api/snispoof/test` and real traffic
  before relying on it. A node that reaches blocked destinations only through
  another tunnel (not a directly DPI-exposed link) may not benefit.
- Pick a `firewall_mark` / `route_table` that don't collide with other policy
  routing on the host (failover/VPN). Defaults are `7137`.
