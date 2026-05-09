# Xray Stack Deployment

This package is designed for a staged migration. Start locked, verify read-only status, then enable write actions only when you are ready to let the Go daemon manage Xray.

## Install Files

```bash
install -m 0755 bin/xray-stackd /usr/local/bin/xray-stackd
install -d /usr/local/share/xray-stack-ui
rsync -a --delete ui/ /usr/local/share/xray-stack-ui/
install -d /usr/local/etc/xray-stack
install -m 0600 config/stack.json /usr/local/etc/xray-stack/stack.json
install -m 0644 systemd/xray-stackd.service /etc/systemd/system/xray-stackd.service
```

Use the imported live stack config as `config/stack.json`; do not deploy `stack.example.json` as-is.

If the legacy Python panel is still bound to `127.0.0.1:8090`, run the Go daemon side-by-side on `127.0.0.1:8091` first:

```bash
jq '.server.admin_listen="127.0.0.1:8091"' config/stack.json > /tmp/stack-go-sidecar.json
install -m 0600 /tmp/stack-go-sidecar.json /usr/local/etc/xray-stack/stack.json
```

## Locked First Start

```bash
cat >/etc/default/xray-stackd <<'EOF'
XRAY_STACKD_FLAGS=
EOF
systemctl daemon-reload
systemctl enable --now xray-stackd.service
systemctl status xray-stackd.service --no-pager
curl -fsS http://127.0.0.1:8090/api/config/summary | jq .
curl -fsS http://127.0.0.1:8090/api/xray/apply-plan | jq .
```

In locked mode, the panel can inspect config, usage, tunnels, quota plans, and bandwidth plans. It cannot modify live Xray or nft/tc state.

## Side-by-Side Nginx Exposure

Keep the legacy panel on `/monitor/` until the Go panel has been verified. The staged route used on ZeroOne is:

- `/monitor-go/` -> `http://127.0.0.1:8091/`
- `/api/` -> `http://127.0.0.1:8091/api/`
- `/assets/` -> `http://127.0.0.1:8091/assets/`

All three locations must keep the same Basic Auth file as `/monitor/`. Run `nginx -t` before reload.

## Enable Writes

Only after the generated config validates and the UI looks correct:

```bash
cat >/etc/default/xray-stackd <<'EOF'
XRAY_STACKD_FLAGS=-allow-apply
EOF
systemctl restart xray-stackd.service
```

Expected write operations:

- `POST /api/xray/apply`: validates generated Xray config, backs up current config, writes atomically, restarts `xray.service`.
- `POST /api/quota/apply`: disables over-quota users in stack config, saves stack config, then applies Xray.
- `POST /api/bandwidth/apply`: applies generated `nft` and `tc` per-user speed rules.

## Rollback

```bash
systemctl stop xray-stackd.service
systemctl disable xray-stackd.service
systemctl restart xray.service
nft delete table inet xray_bw 2>/dev/null || true
tc qdisc del dev eth0 root 2>/dev/null || true
tc qdisc del dev eth0 clsact 2>/dev/null || true
tc qdisc replace dev eth0 root fq
```

Xray config backups created by the Go daemon are stored under `/root/xray-audit-backups/*-go-apply/config.json`.

## Preflight Checklist

- `go run ./cmd/xray-stackd -config config/stack.local.json -print-xray` matches the live Xray snapshot before any speed-limit inbounds are added.
- `scripts/check.sh` passes locally.
- `scripts/build.sh` produces `dist/xray-stackd` and `web/app/dist`.
- Existing ports `443`, `8088`, `1080`, and SSH/VNC access remain unchanged.
- First production start uses locked mode with no `-allow-apply`.
