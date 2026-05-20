# Zeroone Deployment

This package now represents the production Go control plane. Start locked for first installs, then enable write actions when the Go daemon should manage Xray.

## Install Files

```bash
install -m 0755 bin/zeroone /usr/local/bin/zeroone
install -d /usr/local/share/zeroone-ui
rsync -a --delete ui/ /usr/local/share/zeroone-ui/
install -d /usr/local/etc/zeroone
install -m 0600 config/stack.json /usr/local/etc/zeroone/stack.json
install -m 0644 systemd/zeroone.service /etc/systemd/system/zeroone.service
```

Use the imported live stack config as `config/stack.json`; do not deploy `stack.example.json` as-is.

The production daemon listens on `127.0.0.1:8091`:

```bash
jq '.server.admin_listen="127.0.0.1:8091"' config/stack.json > /tmp/stack-go.json
install -m 0600 /tmp/stack-go.json /usr/local/etc/zeroone/stack.json
```

## Locked First Start

```bash
cat >/etc/default/zeroone <<'EOF'
ZEROONE_FLAGS=
EOF
systemctl daemon-reload
systemctl enable --now zeroone.service
systemctl status zeroone.service --no-pager
curl -fsS http://127.0.0.1:8091/api/config/summary | jq .
curl -fsS http://127.0.0.1:8091/api/xray/apply-plan | jq .
```

In locked mode, the panel can inspect config, usage, tunnels, quota plans, and bandwidth plans. It cannot modify live Xray or nft/tc state.

## Side-by-Side Nginx Exposure

The Zeroone production route is:

- `/monitor-go/` -> `http://127.0.0.1:8091/`
- `/api/` -> `http://127.0.0.1:8091/api/`
- `/assets/` -> `http://127.0.0.1:8091/assets/`
- `/monitor/` -> `http://127.0.0.1:8091/`

All three locations must keep the same Basic Auth file as `/monitor/`. Run `nginx -t` before reload.

## Enable Writes

Only after the generated config validates and the UI looks correct:

```bash
cat >/etc/default/zeroone <<'EOF'
ZEROONE_FLAGS=-allow-apply -manage-failover -manage-vpn
EOF
systemctl restart zeroone.service
```

Expected write operations:

- `POST /api/xray/apply`: validates generated Xray config, backs up current config, writes atomically, restarts `xray.service`.
- `POST /api/quota/apply`: disables over-quota users in stack config, saves stack config, then applies Xray.
- `POST /api/bandwidth/apply`: applies generated `nft` and `tc` per-user speed rules.

## Rollback

```bash
systemctl stop zeroone.service
systemctl disable zeroone.service
systemctl restart xray.service
nft delete table inet xray_bw 2>/dev/null || true
tc qdisc del dev eth0 root 2>/dev/null || true
tc qdisc del dev eth0 clsact 2>/dev/null || true
tc qdisc replace dev eth0 root fq
```

Xray config backups created by the Go daemon are stored under `/root/xray-audit-backups/*-go-apply/config.json`.

## Preflight Checklist

- `go run ./cmd/zeroone -config config/stack.local.json -print-xray` matches the live Xray snapshot before any speed-limit inbounds are added.
- `scripts/check.sh` passes locally.
- `scripts/build.sh` produces `dist/zeroone` and `web/app/dist`.
- Existing ports `443`, `8088`, `1080`, and SSH/VNC access remain unchanged.
- Zeroone production currently runs with `-allow-apply -manage-failover -manage-vpn`.
