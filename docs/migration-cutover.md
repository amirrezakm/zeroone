# Migration Cutover

This is the intended production cutover path after the Go daemon reaches feature parity.

## Current Production State

- `xray-stackd` is deployed side by side on `127.0.0.1:8091`.
- nginx routes both `/monitor/` and `/monitor-go/` to the Go daemon.
- The old Python monitor is disabled; nginx no longer uses `127.0.0.1:8090`.
- The old shell AI failover service is disabled; `xray-stackd -manage-failover` owns failover decisions.
- The old shell VPN monitor is disabled; `xray-stackd -manage-vpn` owns tunnel service/IPv4 recovery.
- `allow_apply` is disabled by default; enable it only during a controlled change window.
- The apply pipeline was validated with a temporary user add/apply/delete/apply cycle, then locked again.
- Failover confirmation/cooldown state is persisted in `/var/lib/xray-stack/failover-state.json`.

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

1. Enable apply mode with `XRAY_STACKD_FLAGS=-allow-apply` and restart `xray-stackd`.
2. Apply generated Xray config through `xray-stackd` and verify Xray is active.
3. Verify inbound ports `443`, `1080`, and nginx port `80`.
4. Stop old mutation services only after the Go daemon owns the equivalent behavior.
5. Disable apply mode again unless active panel-side Xray mutation is intentionally needed.
6. Watch `xray`, `xray-stackd`, nginx, and tunnel health logs for 5 minutes.

## Rollback

1. Stop `xray-stackd`.
2. Restore last `/root/xray-audit-backups/.../config.json`.
3. Restart `xray`.
4. Re-enable old services.
