#!/bin/bash
set -u
case "${dev:-}" in
  tun0) metric=10 ;;
  tun1) metric=20 ;;
  *) metric=100 ;;
esac
CIDRS="173.245.48.0/20 103.21.244.0/22 103.22.200.0/22 103.31.4.0/22 141.101.64.0/18 108.162.192.0/18 190.93.240.0/20 188.114.96.0/20 197.234.240.0/22 198.41.128.0/17 162.158.0.0/15 104.16.0.0/13 104.24.0.0/14 172.64.0.0/13 131.0.72.0/22"

apply_routes() {
  for CIDR in $CIDRS; do
    ip route del "$CIDR" dev "$dev" proto boot 2>/dev/null || true
    ip route replace "$CIDR" dev "$dev" metric "$metric" proto static 2>/dev/null || true
  done
}

apply_routes
(
  for delay in 2 5 10 20; do
    sleep "$delay"
    apply_routes
  done
  systemctl try-restart xray.service >/dev/null 2>&1 || true
) &
