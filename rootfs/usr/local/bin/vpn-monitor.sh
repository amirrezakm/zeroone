#!/bin/bash

LOG="/var/log/vpn-monitor.log"
OPENVPN_SERVICE="openvpn@company"
CHECK_INTERVAL=30
RESTART_COOLDOWN=180
last_openvpn_restart=0
last_xray_restart=0

log() {
    echo "[$(date "+%Y-%m-%d %H:%M:%S")] $1" >> "$LOG"
}

restart_openvpn() {
    local now
    now=$(date +%s)
    if [ $((now - last_openvpn_restart)) -lt "$RESTART_COOLDOWN" ]; then
        return 0
    fi
    log "FIX: restarting $OPENVPN_SERVICE"
    systemctl restart "$OPENVPN_SERVICE"
    sleep 15
    log "INFO: xray restart skipped; tunnel failover service owns proxy route changes"
    last_openvpn_restart=$(date +%s)
}

restart_xray() {
    local now
    now=$(date +%s)
    if [ $((now - last_xray_restart)) -lt "$RESTART_COOLDOWN" ]; then
        return 0
    fi
    log "INFO: xray is not active; restart skipped because xray.service has Restart=always"
    last_xray_restart=$(date +%s)
}

log "VPN monitor started for $OPENVPN_SERVICE"

while true; do
    if ! systemctl is-active "$OPENVPN_SERVICE" --quiet; then
        log "PROBLEM: $OPENVPN_SERVICE is not active"
        restart_openvpn
    elif ! ip -4 addr show dev tun0 scope global >/dev/null 2>&1; then
        log "PROBLEM: tun0 has no IPv4 address"
        restart_openvpn
    fi

    if ! systemctl is-active xray.service --quiet; then
        log "PROBLEM: xray is not active"
        restart_xray
    fi

    sleep "$CHECK_INTERVAL"
done
