#!/usr/bin/env bash
set -euo pipefail
python3 -m py_compile rootfs/usr/local/bin/xray-stack-monitor.py
python3 -m py_compile rootfs/usr/local/bin/xray-bandwidth-limits.py
python3 -m py_compile rootfs/var/www/sub/sub.py
bash -n rootfs/usr/local/bin/xray-ai-route-failover.sh
bash -n rootfs/usr/local/bin/vpn-monitor.sh
bash -n rootfs/usr/local/bin/vpn-keepalive.sh
