# Migration Cutover Draft

This is the intended production cutover path after the Go daemon reaches feature parity.

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

1. Stop old mutation services: `xray-stack-monitor`, `xray-ai-route-failover`, `vpn-monitor`.
2. Apply generated Xray config through `xray-stackd` apply pipeline.
3. Restart Xray and verify inbound ports `443`, `1080`, `8088`, monitor route.
4. Switch nginx monitor upstream to the Go daemon.
5. Enable `xray-stackd` and watch logs for 5 minutes.

## Rollback

1. Stop `xray-stackd`.
2. Restore last `/root/xray-audit-backups/.../config.json`.
3. Restart `xray`.
4. Re-enable old services.
