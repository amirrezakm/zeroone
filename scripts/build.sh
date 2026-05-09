#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
mkdir -p "$ROOT/dist"
GOOS=${GOOS:-linux} GOARCH=${GOARCH:-amd64} go build -trimpath -ldflags='-s -w' -o "$ROOT/dist/xray-stackd" ./cmd/xray-stackd
npm --prefix "$ROOT/web/app" run build
