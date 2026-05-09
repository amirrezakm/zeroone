#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
install -m 0755 "$ROOT/dist/xray-stackd" "$ROOT/rootfs/usr/local/bin/xray-stackd"
install -d "$ROOT/rootfs/usr/local/etc/xray-stack"
install -m 0600 "$ROOT/config/stack.local.json" "$ROOT/rootfs/usr/local/etc/xray-stack/stack.json"
install -m 0644 "$ROOT/deploy/systemd/xray-stackd.service" "$ROOT/rootfs/etc/systemd/system/xray-stackd.service"
install -d "$ROOT/rootfs/usr/local/share/xray-stack-ui"
rsync -a --delete "$ROOT/web/app/dist/" "$ROOT/rootfs/usr/local/share/xray-stack-ui/"
