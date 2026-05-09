#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
mkdir -p "$ROOT/dist"
npm --prefix "$ROOT/web/app" run build
GOOS=${GOOS:-linux} GOARCH=${GOARCH:-amd64} go build -trimpath -ldflags='-s -w' -o "$ROOT/dist/xray-stackd" ./cmd/xray-stackd
