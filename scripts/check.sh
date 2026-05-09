#!/usr/bin/env bash
set -euo pipefail
python3 -m py_compile rootfs/usr/local/bin/xray-stack-monitor.py
python3 -m py_compile rootfs/usr/local/bin/xray-bandwidth-limits.py
python3 -m py_compile rootfs/var/www/sub/sub.py
bash -n rootfs/usr/local/bin/xray-ai-route-failover.sh
bash -n rootfs/usr/local/bin/vpn-monitor.sh
bash -n rootfs/usr/local/bin/vpn-keepalive.sh
python3 scripts/import-live-stack.py >/dev/null
go test ./...
go run ./cmd/xray-stackd -config config/stack.local.json -print-xray >/tmp/xray-stackd-generated.json
python3 -m json.tool /tmp/xray-stackd-generated.json >/dev/null
if [ -d web/app/node_modules ]; then
  npm --prefix web/app run build >/dev/null
fi
