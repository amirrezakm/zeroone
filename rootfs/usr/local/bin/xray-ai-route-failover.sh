#!/bin/bash
set -u

CONFIG=/usr/local/etc/xray/config.json
LOG=/var/log/xray-ai-route-failover.log
LOCK=/run/xray-ai-route-failover.lock
STATE=/run/xray-ai-route-failover.state
PRIMARY_IF=tun0
BACKUP_IF=tun1
PRIMARY_TAG=proxy
FALLBACK_TAG=priority-proxy
UPSTREAM_URL=https://edge.velamkon.lol/edge
UPSTREAM_HOST=edge.velamkon.lol
UPSTREAM_IP=172.64.155.209
CHANGE_CONFIRMATIONS=4
MIN_CHANGE_INTERVAL=300

iface_ipv4() {
  ip -4 -o addr show dev "$1" scope global 2>/dev/null | awk '{split($4,a,"/"); print a[1]; exit}'
}

has_ipv4() {
  [ -n "$(iface_ipv4 "$1")" ]
}

healthy_tunnel() {
  local dev="$1" src
  src=$(iface_ipv4 "$dev")
  [ -n "$src" ] || return 1
  timeout 4 nc -z -w 3 -s "$src" "$UPSTREAM_IP" 443 >/dev/null 2>&1
}

choose_mode() {
  if healthy_tunnel "$PRIMARY_IF"; then
    echo "$PRIMARY_TAG:$PRIMARY_IF"
  elif healthy_tunnel "$BACKUP_IF"; then
    echo "$PRIMARY_TAG:$BACKUP_IF"
  else
    echo "$FALLBACK_TAG:"
  fi
}

current_ai_tag() {
  jq -r 'first(.routing.rules[] | select((.domain // []) | index("domain:chatgpt.com"))) | .outboundTag // empty' "$CONFIG"
}

current_proxy_if() {
  jq -r 'first(.outbounds[] | select(.tag == "proxy")) | .streamSettings.sockopt.interface // empty' "$CONFIG"
}

current_mode() {
  local tag iface
  tag=$(current_ai_tag)
  iface=$(current_proxy_if)
  if [ "$tag" = "$PRIMARY_TAG" ]; then
    echo "$tag:$iface"
  else
    echo "$tag:"
  fi
}

confirmed_mode() {
  local desired="$1"
  local current candidate="" count=0 last_change=0 now
  now=$(date +%s)
  current=$(current_mode)
  if [ "$desired" = "$current" ]; then
    echo "$desired 0"
    if [ -r "$STATE" ]; then
      . "$STATE" 2>/dev/null || true
    fi
    printf 'candidate=%s\ncount=%s\nlast_change=%s\n' "" 0 "${last_change:-0}" > "$STATE"
    return 0
  fi
  if [ -r "$STATE" ]; then
    . "$STATE" 2>/dev/null || true
  fi
  if [ $((now - ${last_change:-0})) -lt "$MIN_CHANGE_INTERVAL" ]; then
    echo "$current cooldown"
    return 0
  fi
  if [ "${candidate:-}" = "$desired" ]; then
    count=$(( ${count:-0} + 1 ))
  else
    candidate="$desired"
    count=1
  fi
  printf 'candidate=%s\ncount=%s\nlast_change=%s\n' "$candidate" "$count" "${last_change:-0}" > "$STATE"
  if [ "$count" -lt "$CHANGE_CONFIRMATIONS" ]; then
    echo "$current $count"
    return 0
  fi
  echo "$desired $count"
}

apply_mode() {
  local desired_tag="$1"
  local desired_if="$2"
  local current_tag current_if
  current_tag=$(current_ai_tag)
  current_if=$(current_proxy_if)
  if [ "$current_tag" = "$desired_tag" ] && { [ "$desired_tag" != "$PRIMARY_TAG" ] || [ "$current_if" = "$desired_if" ]; }; then
    return 0
  fi

  local stamp backup tmp
  stamp=$(date +%Y%m%d-%H%M%S-ai-route-${desired_tag}-${desired_if:-none})
  backup=/root/xray-audit-backups/$stamp
  tmp=/tmp/xray-ai-route-$stamp.json
  mkdir -p "$backup"
  cp -a "$CONFIG" "$backup/config.json"

  if [ "$desired_tag" = "$PRIMARY_TAG" ]; then
    jq --arg tag "$desired_tag" --arg iface "$desired_if" '
      (.routing.rules[] | select((.domain // []) | index("domain:chatgpt.com")) | .outboundTag) = $tag
      | (.outbounds[] | select(.tag == "proxy") | .streamSettings.sockopt.interface) = $iface
    ' "$CONFIG" > "$tmp"
  else
    jq --arg tag "$desired_tag" '
      (.routing.rules[] | select((.domain // []) | index("domain:chatgpt.com")) | .outboundTag) = $tag
    ' "$CONFIG" > "$tmp"
  fi

  if ! xray run -test -config "$tmp" >/tmp/xray-ai-route-test.out 2>&1; then
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] config test failed for tag=$desired_tag iface=${desired_if:-none}" >> "$LOG"
    cat /tmp/xray-ai-route-test.out >> "$LOG"
    return 1
  fi
  install -m 0644 "$tmp" "$CONFIG"
  systemctl restart xray.service
  if [ -r "$STATE" ]; then
    . "$STATE" 2>/dev/null || true
  fi
  printf 'candidate=%s\ncount=%s\nlast_change=%s\n' "" 0 "$(date +%s)" > "$STATE"
  echo "[$(date +'%Y-%m-%d %H:%M:%S')] ai tag ${current_tag:-none} -> $desired_tag, proxy iface ${current_if:-none} -> ${desired_if:-none}, backup=$backup" >> "$LOG"
}

run_once() {
  local mode confirmed confirm_count desired_tag desired_if
  mode=$(choose_mode)
  read -r confirmed confirm_count < <(confirmed_mode "$mode")
  desired_tag=${confirmed%%:*}
  desired_if=${confirmed#*:}
  if [ "$confirmed" != "$mode" ]; then
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] pending route change desired=$mode confirmation=$confirm_count/$CHANGE_CONFIRMATIONS current=$confirmed" >> "$LOG"
  fi
  apply_mode "$desired_tag" "$desired_if"
}

if [ "${1:-}" = "--once" ]; then
  exec 9>"$LOCK"
  flock -n 9 || exit 0
  run_once
  exit 0
fi

while true; do
  exec 9>"$LOCK"
  if flock -n 9; then
    run_once || echo "[$(date +'%Y-%m-%d %H:%M:%S')] route check failed" >> "$LOG"
  fi
  sleep 15
done
