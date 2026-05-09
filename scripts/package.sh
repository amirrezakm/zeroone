#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
VERSION=${VERSION:-$(git -C "$ROOT" rev-parse --short HEAD)}
PKG_DIR="$ROOT/dist/package/xray-stack-zeroone-$VERSION"
ARCHIVE="$ROOT/dist/xray-stack-zeroone-$VERSION.tar.gz"

"$ROOT/scripts/build.sh"
rm -rf "$PKG_DIR"
mkdir -p "$PKG_DIR"/{bin,systemd,ui,config,docs}

install -m 0755 "$ROOT/dist/xray-stackd" "$PKG_DIR/bin/xray-stackd"
install -m 0644 "$ROOT/deploy/systemd/xray-stackd.service" "$PKG_DIR/systemd/xray-stackd.service"
cp -a "$ROOT/web/app/dist/." "$PKG_DIR/ui/"
install -m 0644 "$ROOT/config/stack.example.json" "$PKG_DIR/config/stack.example.json"
install -m 0644 "$ROOT/deploy/DEPLOY.md" "$PKG_DIR/docs/DEPLOY.md"

tar -C "$ROOT/dist/package" -czf "$ARCHIVE" "xray-stack-zeroone-$VERSION"
printf '%s\n' "$ARCHIVE"
