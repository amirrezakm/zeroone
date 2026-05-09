#!/bin/bash
if ip link show tun0 | grep -q UP; then
    curl --interface tun0 -s -o /dev/null --max-time 5 https://chatgpt.com
fi
