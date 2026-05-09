#!/usr/bin/env bash
set -euo pipefail
SERVER=${SERVER:-root@185.128.139.68}
KEY=${KEY:-$HOME/.ssh/id_ed25519}
PROJECT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
SSH_OPTS="-F /dev/null -o ConnectTimeout=8 -o IdentitiesOnly=yes -i $KEY"

rsync -az --delete \
  -e "ssh $SSH_OPTS" \
  --relative \
  --exclude='__pycache__/' \
  --exclude='*.pyc' \
  --exclude='/usr/local/bin/xray' \
  "$SERVER":/usr/local/bin/xray-stack-monitor.py \
  "$SERVER":/usr/local/bin/xray-ai-route-failover.sh \
  "$SERVER":/usr/local/bin/vpn-monitor.sh \
  "$SERVER":/usr/local/bin/vpn-keepalive.sh \
  "$SERVER":/usr/local/bin/xray-bandwidth-limits.py \
  "$SERVER":/usr/local/etc/xray/ \
  "$SERVER":/etc/systemd/system/xray.service \
  "$SERVER":/etc/systemd/system/xray.service.d/override.conf \
  "$SERVER":/etc/systemd/system/xray-stack-monitor.service \
  "$SERVER":/etc/systemd/system/xray-ai-route-failover.service \
  "$SERVER":/etc/systemd/system/vpn-monitor.service \
  "$SERVER":/etc/systemd/system/sub.service \
  "$SERVER":/etc/systemd/system/openvpn@company.service.d/override.conf \
  "$SERVER":/etc/openvpn/ \
  "$SERVER":/etc/nginx/nginx.conf \
  "$SERVER":/etc/nginx/conf.d/ \
  "$SERVER":/etc/nginx/sites-available/ \
  "$SERVER":/var/www/sub/ \
  "$PROJECT_DIR/rootfs/"
