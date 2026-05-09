#!/usr/bin/env bash
set -euo pipefail
python3 scripts/import-live-stack.py >/dev/null
go test ./...
go run ./cmd/xray-stackd -config config/stack.local.json -print-xray >/tmp/xray-stackd-generated.json
python3 -m json.tool /tmp/xray-stackd-generated.json >/dev/null
if [ -d web/app/node_modules ]; then
  npm --prefix web/app run build >/dev/null
fi
