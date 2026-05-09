# xray-stack-zeroone

Local development snapshot for the ZeroOne Xray stack on `185.128.139.68`.

## Layout

- `rootfs/`: server filesystem snapshot, kept in deployable paths.
- `rootfs/usr/local/bin/xray-stack-monitor.py`: web monitor and admin panel.
- `rootfs/usr/local/bin/xray-ai-route-failover.sh`: AI route failover controller.
- `rootfs/usr/local/bin/vpn-monitor.sh`: tunnel monitoring script.
- `rootfs/usr/local/etc/xray/config.json`: live Xray config snapshot. Sensitive; ignored by git.
- `rootfs/etc/openvpn/`: OpenVPN configs and route hooks. Sensitive; ignored by git.
- `rootfs/etc/systemd/system/`: service units and overrides.
- `rootfs/etc/nginx/`: nginx config for monitor/subscription exposure.
- `rootfs/var/www/sub/`: subscription server.
- `config/examples/config.example.json`: sanitized example Xray config.

## Development Workflow

1. Edit locally under `rootfs/`.
2. Run syntax checks:
   - `python3 -m py_compile rootfs/usr/local/bin/xray-stack-monitor.py`
   - `bash -n rootfs/usr/local/bin/xray-ai-route-failover.sh`
   - `bash -n rootfs/usr/local/bin/vpn-monitor.sh`
3. Sync from server only when intentionally refreshing the snapshot:
   - `scripts/sync-from-server.sh`
4. Deploy intentionally with a reviewed diff. Do not rsync secrets blindly into another machine.

## Notes

This repo currently contains a local snapshot for development. Real credentials, usage counters, OpenVPN auth files, and live Xray configs are present on disk under `rootfs/` for local reference but are excluded from git by default.
