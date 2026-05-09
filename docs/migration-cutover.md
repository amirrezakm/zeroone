# Migration Cutover

Production cutover was completed on ZeroOne (`185.128.139.68`) on `2026-05-10 04:32 +0330`.

## Current Production State

- `xray-stackd` is the production control plane on `127.0.0.1:8091`.
- nginx routes both `/monitor/` and `/monitor-go/` to the Go daemon.
- The old Python monitor is disabled; nginx no longer uses `127.0.0.1:8090`.
- The old shell AI failover service is masked; `xray-stackd -manage-failover` owns failover decisions.
- The old shell VPN monitor is masked; `xray-stackd -manage-vpn` owns tunnel service/IPv4 recovery.
- Legacy services are masked with `/etc/systemd/system/*.service -> /dev/null`: `vpn-monitor`, `xray-stack-monitor`, `xray-ai-route-failover`, `xray2`, `xray-icmp`, and `xray-bandwidth-limits`.
- The legacy Python subscription service `sub.service` is also masked and its nginx site is disabled.
- Legacy executable scripts were moved out of `/usr/local/bin` and archived under `/root/xray-audit-backups/legacy-files-removed-20260510-044238`.
- `allow_apply` is enabled in `/etc/default/xray-stackd`, so the Go panel owns production Xray mutations.
- The apply pipeline was validated with a temporary direct rule add/apply/delete/apply cycle after cutover.
- Failover confirmation/cooldown state is persisted in `/var/lib/xray-stack/failover-state.json`.
- Current rollback metadata is recorded on the server at `/root/xray-audit-backups/go-cutover-current.txt`.
- The failover state path is explicit in stack config: `/var/lib/xray-stack/failover-state.json`.

## Preflight

1. Pull a fresh server snapshot: `scripts/sync-from-server.sh`.
2. Import live source-of-truth: `scripts/import-live-stack.py`.
3. Run checks: `scripts/check.sh`.
4. Build Linux binary: `scripts/build.sh`.
5. Compare generated Xray config to live config. Expected diff should be reviewed and intentional.

## Staged Deploy

1. Upload `dist/xray-stackd` to `/usr/local/bin/xray-stackd`.
2. Upload `config/stack.local.json` to `/usr/local/etc/xray-stack/stack.json` with mode `0600`.
3. Install `deploy/systemd/xray-stackd.service`.
4. Start the new daemon on `127.0.0.1:8091` first if running beside the old panel.
5. Validate read-only endpoints: `/api/health`, `/api/config/summary`, `/api/xray/generated`, `/api/failover/decision`.

## Cutover Window

Allowed downtime: up to 10 minutes.

1. Enable apply mode with `XRAY_STACKD_FLAGS=-allow-apply -manage-failover -manage-vpn` and restart `xray-stackd`.
2. Apply generated Xray config through `xray-stackd` and verify Xray is active.
3. Verify inbound ports `443`, `1080`, and nginx port `80`.
4. Stop old mutation services only after the Go daemon owns the equivalent behavior.
5. Keep apply mode enabled only when the panel is intended to own production mutations.
6. Watch `xray`, `xray-stackd`, nginx, and tunnel health logs for 5 minutes.

## Rollback

1. Read `/root/xray-audit-backups/go-cutover-current.txt`.
2. Restore `/usr/local/etc/xray-stack`, `/usr/local/etc/xray`, and `/var/lib/xray-stack` from `pre_cutover_backup` if needed.
3. Replace legacy unit symlinks with files from `legacy_unit_archive` only if rolling back to the old shell/Python control plane.
4. Restart `xray`, `xray-stackd`, nginx, and the OpenVPN units.
