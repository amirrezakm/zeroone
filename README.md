# xray-stack-zeroone

Local development snapshot for the ZeroOne Xray stack on `185.128.139.68`.

## Layout

- `cmd/xray-stackd/`: Go control plane for the panel, Xray apply, failover, and VPN supervision.
- `web/app/`: production panel UI served by `xray-stackd`.
- `config/stack.local.json`: imported production stack config for local development.
- `rootfs/`: reduced server filesystem snapshot for nginx, OpenVPN, and systemd reference.
- `rootfs/usr/local/etc/xray/config.json`: live Xray config snapshot. Sensitive; ignored by git.
- `rootfs/etc/openvpn/`: OpenVPN configs and route hooks. Sensitive; ignored by git.
- `rootfs/etc/systemd/system/`: service units and overrides.
- `rootfs/etc/nginx/`: nginx config for Go panel exposure.
- `config/examples/config.example.json`: sanitized example Xray config.

## Development Workflow

1. Edit Go/backend code, UI, or stack config locally.
2. Run syntax checks:
   - `scripts/check.sh`
3. Sync from server only when intentionally refreshing the snapshot:
   - `scripts/sync-from-server.sh`
4. Deploy intentionally with a reviewed diff. Do not rsync secrets blindly into another machine.

## Notes

This repo currently contains a local snapshot for development. Real credentials, usage counters, OpenVPN auth files, and live Xray configs are present on disk under `rootfs/` for local reference but are excluded from git by default.
