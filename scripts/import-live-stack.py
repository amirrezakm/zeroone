#!/usr/bin/env python3
import json
from pathlib import Path

root = Path(__file__).resolve().parents[1]
xray_config = json.loads((root / 'rootfs/usr/local/etc/xray/config.json').read_text())
try:
    bandwidth_limits = json.loads((root / 'rootfs/usr/local/etc/xray/bandwidth-limits.json').read_text())
except Exception:
    bandwidth_limits = {}

def first_out(tag):
    return next(o for o in xray_config.get('outbounds', []) if o.get('tag') == tag)

def outbound_model(tag):
    o = first_out(tag)
    vnext = o.get('settings', {}).get('vnext', [{}])[0]
    user = (vnext.get('users') or [{}])[0]
    stream = o.get('streamSettings', {})
    tls = stream.get('tlsSettings', {})
    ws = stream.get('wsSettings', {})
    return {
        'tag': tag,
        'type': 'vless-ws-tls' if stream.get('security') == 'tls' else 'vless-ws',
        'address': vnext.get('address', ''),
        'port': vnext.get('port', 0),
        'uuid': user.get('id', ''),
        'server_name': tls.get('serverName', ''),
        'host': ws.get('host', ''),
        'path': ws.get('path', '/'),
        'interface': stream.get('sockopt', {}).get('interface', ''),
        'mux_concurrency': int((o.get('mux') or {}).get('concurrency') or 0),
    }

users = []
for inbound in xray_config.get('inbounds', []):
    if inbound.get('protocol') == 'vless' and inbound.get('port') == 443:
        for c in inbound.get('settings', {}).get('clients', []):
            email = c.get('email', '')
            limit = bandwidth_limits.get(email, {}) if isinstance(bandwidth_limits, dict) else {}
            users.append({
                'email': email,
                'uuid': c.get('id', ''),
                'enabled': True,
                'quota_bytes': int(c.get('quota_bytes') or 0),
                'download_mbps': int(float(limit.get('download_mbps') or 0)),
                'upload_mbps': int(float(limit.get('upload_mbps') or 0)),
                'bandwidth_port': int(limit.get('port') or 0),
            })
        break

socks = []
for inbound in xray_config.get('inbounds', []):
    if inbound.get('protocol') == 'socks' and inbound.get('listen') == '0.0.0.0':
        account = (inbound.get('settings', {}).get('accounts') or [{}])[0]
        socks.append({
            'name': inbound.get('tag', 'socks').replace('managed-socks-', ''),
            'listen': inbound.get('listen', '0.0.0.0'),
            'port': inbound.get('port', 0),
            'username': account.get('user', ''),
            'password': account.get('pass', ''),
        })

rules = xray_config.get('routing', {}).get('rules', [])
def domain_rules_for(tag):
    return [r.get('domain', []) for r in rules if r.get('outboundTag') == tag and r.get('domain')]
def ip_rules_for(tag):
    return [r.get('ip', []) for r in rules if r.get('outboundTag') == tag and r.get('ip')]

direct_domain_rules = domain_rules_for('direct')
direct_ip_rules = ip_rules_for('direct')
block_domain_rules = domain_rules_for('block')
block_ip_rules = ip_rules_for('block')
proxy_domain_rules = domain_rules_for('proxy')

stack = {
    'server': {
        'public_ip': '185.128.139.68',
        'admin_listen': '127.0.0.1:8090',
        'xray_config_path': '/usr/local/etc/xray/config.json',
        'xray_binary': '/usr/local/bin/xray',
        'backup_dir': '/root/xray-audit-backups',
        'user_usage_path': '/usr/local/etc/xray/user-usage.json',
        'socks_usage_path': '/usr/local/etc/xray/socks-usage.json',
        'ui_path': '/usr/local/share/xray-stack-ui',
        'bandwidth_device': 'eth0',
        'bandwidth_config_path': '/usr/local/etc/xray/bandwidth-limits.json',
        'failover_state_path': '/var/lib/xray-stack/failover-state.json',
    },
    'xray': {
        'log_level': xray_config.get('log', {}).get('loglevel', 'warning'),
        'dns_servers': xray_config.get('dns', {}).get('servers', []),
        'dns_hosts': xray_config.get('dns', {}).get('hosts', {}),
        'api_port': 10085,
        'inbounds': {'vless_ws_port': 443, 'vless_xhttp_port': 3002, 'local_socks_port': 10808, 'public_socks': socks},
        'users': users,
        'outbounds': {'proxy': outbound_model('proxy'), 'fallback': outbound_model('priority-proxy')},
        'routing': {
            'block_udp_443': any(r.get('network') == 'udp' and r.get('port') == '443' and r.get('outboundTag') == 'block' for r in rules),
            'direct_domains': direct_domain_rules[0] if direct_domain_rules else [],
            'direct_ips': direct_ip_rules[0] if direct_ip_rules else [],
            'block_domains': block_domain_rules[0] if len(block_domain_rules) > 0 else [],
            'manual_block_domains': block_domain_rules[1] if len(block_domain_rules) > 1 else [],
            'block_ips': block_ip_rules[0] if block_ip_rules else [],
            'ai_update_domains': proxy_domain_rules[0] if len(proxy_domain_rules) > 1 else [],
            'ai_domains': proxy_domain_rules[-1] if proxy_domain_rules else [],
        },
    },
    'tunnels': [
        {'name': 'company', 'type': 'openvpn', 'interface': 'tun0', 'systemd_unit': 'openvpn@company', 'priority': 10},
        {'name': 'backup', 'type': 'openvpn', 'interface': 'tun1', 'systemd_unit': 'openvpn@client-tun1', 'priority': 20},
    ],
    'failover': {'enabled': True, 'probe_ip': '172.64.155.209', 'probe_port': 443, 'interval_seconds': 15, 'confirmations': 4, 'cooldown_seconds': 300, 'fallback_outbound_tag': 'priority-proxy'},
}

out = root / 'config/stack.local.json'
out.write_text(json.dumps(stack, indent=2, ensure_ascii=False) + '\n')
print(out)
