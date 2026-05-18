#!/usr/bin/env bash
set -euo pipefail
: "${SERVER:?set SERVER=user@host (e.g. SERVER=root@origin.example.com)}"
KEY=${KEY:-$HOME/.ssh/id_ed25519}
PROJECT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
SSH_OPTS="-F /dev/null -o ConnectTimeout=8 -o IdentitiesOnly=yes -i $KEY"

rsync -az --delete \
  -e "ssh $SSH_OPTS" \
  --relative \
  --exclude='__pycache__/' \
  --exclude='*.pyc' \
  --exclude='/usr/local/bin/xray' \
  "$SERVER":/usr/local/bin/xray-stackd \
  "$SERVER":/usr/local/share/xray-stack-ui/ \
  "$SERVER":/usr/local/etc/xray-stack/ \
  "$SERVER":/usr/local/etc/xray/ \
  "$SERVER":/etc/systemd/system/xray.service \
  "$SERVER":/etc/systemd/system/xray.service.d/override.conf \
  "$SERVER":/etc/systemd/system/xray-stackd.service \
  "$SERVER":/etc/systemd/system/openvpn@company.service.d/override.conf \
  "$SERVER":/etc/openvpn/ \
  "$SERVER":/etc/nginx/nginx.conf \
  "$SERVER":/etc/nginx/conf.d/ \
  "$SERVER":/etc/nginx/sites-available/ \
  "$PROJECT_DIR/rootfs/"
