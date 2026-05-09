#!/usr/bin/env python3
import json
import os
import re
import shlex
import subprocess
import time
import uuid
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, quote, unquote, urlparse

HOST = "127.0.0.1"
PORT = 8090
QUOTA_FILE = "/usr/local/etc/xray/user-quotas.json"
USAGE_FILE = "/usr/local/etc/xray/user-usage.json"
BAN_FILE = "/usr/local/etc/xray/user-bans.json"
BANDWIDTH_FILE = "/usr/local/etc/xray/bandwidth-limits.json"
SOCKS_USAGE_FILE = "/usr/local/etc/xray/socks-usage.json"
SOCKS_PORT_START = 22000
SOCKS_PORT_END = 22999
XRAY_API_SERVER = "127.0.0.1:10085"
EDGE_UPSTREAM_IP = "172.64.155.209"
STATE_LOCK = threading.RLock()
INTERNAL_STATS_USERS = {"router"}
CPU_LOCK = threading.Lock()
CPU_LAST = None
STATUS_CACHE = {
    "xray_config": {"mtime": 0, "ts": 0, "ok": False},
    "traffic": {"ts": 0, "value": None},
    "proxy_upstream": {"ts": 0, "value": None},
    "xray_socks": {"ts": 0, "value": None},
}

HTML = """<!doctype html>
<html lang="en" dir="ltr">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Xray Stack Monitor</title>
  <style>
    :root { color-scheme: light dark; --ok:#16803c; --bad:#b42318; --warn:#b54708; --muted:#667085; --line:#d0d5dd; --bg:#f6f7f9; --card:#fff; --text:#101828; --accent:#175cd3; --soft:#eff4ff; --card-shadow:0 1px 2px rgba(16,24,40,.05), 0 10px 24px rgba(16,24,40,.06); }
    @media (prefers-color-scheme: dark) { :root { --bg:#0f1216; --card:#171b22; --text:#f2f4f7; --line:#303744; --muted:#98a2b3; --card-shadow:0 12px 28px rgba(0,0,0,.22); } }
    * { box-sizing: border-box; }
    body { margin:0; font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background:var(--bg); color:var(--text); }
    header { display:flex; align-items:center; justify-content:space-between; gap:16px; padding:18px 24px; border-bottom:1px solid var(--line); background:var(--card); position:sticky; top:0; z-index:2; }
    h1 { font-size:20px; margin:0; font-weight:700; }
    main { padding:24px; max-width:1180px; margin:auto; }
    .top-actions { display:flex; align-items:center; gap:10px; }
    .tabs { display:flex; gap:8px; margin-bottom:18px; border-bottom:1px solid var(--line); }
    .tab { border:0; border-bottom:2px solid transparent; border-radius:0; padding:10px 12px; background:transparent; color:var(--muted); font-weight:650; }
    .tab.active { color:var(--accent); border-bottom-color:var(--accent); }
    .panel[hidden] { display:none; }
    .section-head { display:flex; align-items:flex-end; justify-content:space-between; gap:14px; margin:0 0 14px; }
    .section-title { margin:0; font-size:18px; }
    .section-note { color:var(--muted); font-size:13px; margin:4px 0 0; }
    .grid { display:grid; grid-template-columns: repeat(auto-fit, minmax(230px, 1fr)); gap:14px; align-items:stretch; }
    .card { background:var(--card); border:1px solid var(--line); border-radius:8px; padding:15px; min-height:104px; box-shadow:var(--card-shadow); }
    .stat-card { display:flex; flex-direction:column; justify-content:space-between; gap:10px; border-top:3px solid color-mix(in srgb, var(--accent) 70%, var(--line)); }
    .stat-top { display:flex; align-items:flex-start; justify-content:space-between; gap:10px; }
    .stat-icon { width:34px; height:34px; border-radius:8px; display:grid; place-items:center; background:var(--soft); color:var(--accent); font-weight:800; flex:0 0 auto; }
    .stat-main { min-width:0; }
    .stat-value { font-size:22px; line-height:1.05; font-weight:760; overflow-wrap:anywhere; }
    .stat-detail { color:var(--muted); font-size:12px; margin-top:5px; overflow-wrap:anywhere; }
    .meter { height:7px; overflow:hidden; border-radius:999px; background:color-mix(in srgb, var(--line) 55%, transparent); }
    .meter-fill { height:100%; width:0%; border-radius:999px; background:var(--accent); }
    .meter-fill.ok { background:var(--ok); } .meter-fill.warn { background:var(--warn); } .meter-fill.bad { background:var(--bad); }
    .wide { grid-column: 1 / -1; }
    .title { color:var(--muted); font-size:13px; margin-bottom:8px; }
    .value { font-size:16px; font-weight:650; text-align:left; overflow-wrap:anywhere; }
    .status { display:inline-flex; align-items:center; gap:8px; direction:ltr; }
    .dot { width:10px; height:10px; border-radius:99px; background:var(--muted); display:inline-block; }
    .ok .dot { background:var(--ok); } .bad .dot { background:var(--bad); } .warn .dot { background:var(--warn); }
    .ok { color:var(--ok); } .bad { color:var(--bad); } .warn { color:var(--warn); }
    pre { margin:0; white-space:pre-wrap; direction:ltr; text-align:left; font-size:12px; line-height:1.45; color:var(--text); }
    button { border:1px solid var(--line); background:var(--card); color:var(--text); border-radius:7px; padding:9px 12px; cursor:pointer; font-weight:600; }
    button.primary { background:var(--accent); border-color:var(--accent); color:white; }
    button.ghost { background:transparent; }
    .test-row { display:flex; align-items:center; justify-content:space-between; gap:10px; margin-top:10px; }
    .form { display:grid; grid-template-columns: repeat(auto-fit, minmax(190px, 1fr)); gap:10px; margin-top:12px; align-items:end; }
    label { display:block; font-size:12px; color:var(--muted); margin-bottom:5px; }
    input, select { width:100%; border:1px solid var(--line); background:var(--bg); color:var(--text); border-radius:7px; padding:9px 10px; direction:ltr; }
    .list { display:flex; flex-wrap:wrap; gap:8px; margin-top:10px; direction:ltr; }
    .pill { display:inline-flex; align-items:center; gap:8px; border:1px solid var(--line); border-radius:999px; padding:6px 9px; background:var(--bg); font-size:12px; direction:ltr; }
    .pill button { padding:2px 6px; border-radius:999px; font-size:11px; }
    .danger { color:var(--bad); }
    button.danger { border-color: color-mix(in srgb, var(--bad) 45%, var(--line)); }
    .small { font-size:12px; color:var(--muted); font-weight:500; text-align:left; margin-top:6px; }
    textarea { width:100%; min-height:82px; resize:vertical; border:1px solid var(--line); background:var(--bg); color:var(--text); border-radius:7px; padding:9px 10px; direction:ltr; font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size:12px; }
    .meta { color:var(--muted); font-size:13px; direction:ltr; text-align:left; }
    .settings-grid { display:grid; grid-template-columns: minmax(0, 1fr); gap:14px; }
    .subhead { font-size:14px; font-weight:700; margin:16px 0 2px; }
    .divider { height:1px; background:var(--line); margin:14px 0; }
    .notice { border:1px solid var(--line); border-radius:8px; padding:12px; background:var(--soft); color:#1849a9; margin-bottom:14px; font-size:13px; }
    .row-list { display:grid; gap:8px; margin-top:10px; }
    .row-item { display:flex; align-items:center; justify-content:space-between; gap:12px; border:1px solid var(--line); border-radius:8px; padding:10px; background:var(--bg); }
    .row-main { min-width:0; }
    .row-title { font-weight:700; overflow-wrap:anywhere; }
    .row-actions { display:flex; gap:8px; flex-shrink:0; }
    .usage { display:flex; gap:10px; flex-wrap:wrap; margin-top:5px; }
    .activity { display:grid; gap:6px; max-height:420px; overflow:auto; }
    .activity-line { display:grid; grid-template-columns: 145px minmax(0, 1fr) 80px 120px; gap:8px; align-items:center; min-width:0; padding:7px 0; border-bottom:1px solid var(--line); }
    .activity-time { color:var(--muted); flex:0 0 auto; }
    .activity-dest { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
    .activity-tag { color:var(--muted); flex:0 0 auto; }
    .modal-backdrop { position:fixed; inset:0; background:rgba(0,0,0,.45); display:none; align-items:center; justify-content:center; padding:18px; z-index:20; }
    .modal { width:min(920px, 100%); max-height:88vh; overflow:hidden; background:var(--card); border:1px solid var(--line); border-radius:8px; box-shadow:0 18px 50px rgba(0,0,0,.25); }
    .modal-head { display:flex; align-items:center; justify-content:space-between; gap:12px; padding:14px 16px; border-bottom:1px solid var(--line); }
    .modal-body { padding:14px 16px; }
    @media (prefers-color-scheme: dark) { .notice { background:#142033; color:#b2ccff; } }
  </style>
</head>
<body>
  <header>
    <h1>Xray Server Monitor</h1>
    <div class="top-actions">
      <button id="refresh" class="ghost">Refresh</button>
    </div>
  </header>
  <main>
    <div class="tabs">
      <button id="tab-status" class="tab active" onclick="showTab('status')">Monitor</button>
      <button id="tab-settings" class="tab" onclick="showTab('settings')">Settings</button>
    </div>
    <section id="panel-status" class="panel">
      <div class="section-head">
        <div>
          <h2 class="section-title">Live status</h2>
          <p class="section-note">Services and routes refresh automatically. External site checks only run when you click Test.</p>
        </div>
        <div id="meta" class="meta">loading...</div>
      </div>
      <div id="status-grid" class="grid"></div>
    </section>
    <section id="panel-settings" class="panel" hidden>
      <div class="section-head">
        <div>
          <h2 class="section-title">Xray settings</h2>
          <p class="section-note">Changes are backed up, validated, and applied with a controlled Xray restart.</p>
        </div>
      </div>
      <div class="notice">Every write validates the Xray config before applying it. Full UUIDs are hidden in lists, but new connection links are shown after user creation.</div>
      <div id="settings-grid" class="settings-grid"></div>
    </section>
  </main>
  <div id="activity-modal" class="modal-backdrop">
    <div class="modal">
      <div class="modal-head">
        <div>
          <div class="title" id="activity-title">Activity</div>
          <div class="small" id="activity-subtitle"></div>
        </div>
        <div class="row-actions">
          <button onclick="refreshActivity()" class="primary">Refresh</button>
          <button onclick="closeActivityModal()" class="ghost">Close</button>
        </div>
      </div>
      <div class="modal-body">
        <div id="activity-list" class="activity"></div>
      </div>
    </div>
  </div>
<script>
const statusGrid = document.getElementById('status-grid');
const settingsGrid = document.getElementById('settings-grid');
const meta = document.getElementById('meta');
let activeTab = 'status';
let currentUsers = [];
let currentSocksUsers = [];
let currentActivityEmail = '';
settingsGrid.innerHTML = adminCard();
function showTab(name){
  activeTab = name;
  document.getElementById('panel-status').hidden = name !== 'status';
  document.getElementById('panel-settings').hidden = name !== 'settings';
  document.getElementById('tab-status').classList.toggle('active', name === 'status');
  document.getElementById('tab-settings').classList.toggle('active', name === 'settings');
  if (name === 'settings') loadAdmin();
}
function cls(v){ return v === 'active' || v === 'ok' || v === 'disabled' || v === true ? 'ok' : (v === 'inactive' || v === 'failed' || v === false ? 'bad' : 'warn'); }
function emoji(v){ return cls(v) === 'ok' ? '✅' : (cls(v) === 'bad' ? '❌' : '⚠️'); }
function esc(v){ return String(v ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])); }
function toneForPercent(value){ return value >= 90 ? 'bad' : (value >= 75 ? 'warn' : 'ok'); }
function statCard(title, value, detail='', icon='•', percent=null, tone=''){
  const pct = percent === null || percent === undefined ? null : Math.max(0, Math.min(100, Number(percent) || 0));
  const barTone = tone || (pct === null ? '' : toneForPercent(pct));
  return `<div class="card stat-card">
    <div class="stat-top">
      <div class="stat-main">
        <div class="title">${esc(title)}</div>
        <div class="stat-value ${esc(barTone)}">${esc(value || '-')}</div>
        <div class="stat-detail">${esc(detail || '')}</div>
      </div>
      <div class="stat-icon">${esc(icon)}</div>
    </div>
    ${pct === null ? '' : `<div class="meter"><div class="meter-fill ${esc(barTone)}" style="width:${pct}%"></div></div>`}
  </div>`;
}
function statusCard(title, value){ const c=cls(value); return `<div class="card stat-card"><div class="stat-top"><div class="stat-main"><div class="title">${esc(title)}</div><div class="value ${c}"><span class="status"><span class="dot"></span>${emoji(value)} ${esc(value)}</span></div></div><div class="stat-icon">${c === 'ok' ? '✓' : (c === 'bad' ? '!' : '?')}</div></div></div>`; }
function textCard(title, value){ return statCard(title, value || '-', '', 'i'); }
function routeCard(route){
  if (!route) return '';
  const c = route.ok ? 'ok' : 'bad';
  return `<div class="card stat-card"><div class="stat-top"><div class="stat-main"><div class="title">AI tunnel failover</div><div class="value ${c}"><span class="status"><span class="dot"></span>${emoji(route.ok)} ${esc(route.state)}</span></div><div class="stat-detail">${esc(route.summary)}</div></div><div class="stat-icon">AI</div></div></div>`;
}
function healthCard(title, check, icon='NET'){
  if (!check) return '';
  const c = check.ok ? 'ok' : 'bad';
  return `<div class="card stat-card"><div class="stat-top"><div class="stat-main"><div class="title">${esc(title)}</div><div class="value ${c}"><span class="status"><span class="dot"></span>${emoji(check.ok)} ${check.ok ? 'ok' : 'failed'}</span></div><div class="stat-detail">${esc(check.detail || '')}</div></div><div class="stat-icon">${esc(icon)}</div></div></div>`;
}
function testCard(key, title){
  return `<div class="card"><div class="title">${esc(title)}</div><div id="test-${key}" class="value warn">⚠️ Not tested</div><div id="test-${key}-detail" class="small">Manual check only.</div><div class="test-row"><button onclick="runTest('${key}')" class="primary">Test</button></div></div>`;
}
function tunnelTestCard(){
  return `<div class="card wide">
    <div class="title">Tunnel destination test</div>
    <div class="small">Test a domain or IP through tun0, tun1, or the server IP.</div>
    <div class="form">
      <div><label>Target</label><input id="tunnel-target" placeholder="example.com or 1.1.1.1"></div>
      <div><label>Port</label><input id="tunnel-port" value="443"></div>
      <div><label>Interface</label><select id="tunnel-interface"><option value="tun0">tun0</option><option value="tun1">tun1</option><option value="eth0">server IP</option></select></div>
      <button onclick="runTunnelTargetTest()" class="primary">Test target</button>
    </div>
    <div id="tunnel-test-result" class="small"></div>
  </div>`;
}
function preCard(title, value){ return `<div class="card wide"><div class="title">${esc(title)}</div><pre>${esc(value || '-')}</pre></div>`; }
function adminCard(){
  return `<div class="card wide">
    <div class="title">VLESS users</div>
    <div class="small">This list is synced across all current VLESS inbounds.</div>
    <div id="usage-meta" class="small"></div>
    <div id="users-list" class="row-list"></div>
    <div class="divider"></div>
    <div class="subhead">Add user</div>
    <div class="form">
      <div><label>Email</label><input id="user-email" placeholder="parastoo"></div>
      <div><label>UUID</label><input id="user-id" placeholder="empty = generate"></div>
      <button onclick="addUser()" class="primary">Add user</button>
    </div>
    <div id="edit-panel" style="display:none">
      <div class="divider"></div>
      <div class="subhead">Edit user</div>
      <div class="form">
        <div><label>Current user</label><input id="edit-user" readonly></div>
        <div><label>New email</label><input id="edit-email" placeholder="leave empty = keep"></div>
        <div><label>New UUID</label><input id="edit-id" placeholder="optional"></div>
        <button onclick="editUser()" class="primary">Save changes</button>
        <button onclick="cancelEdit()" class="ghost">Cancel</button>
      </div>
    </div>
    <div id="users-msg" class="small"></div>
    <div id="user-config" style="display:none; margin-top:12px">
      <div class="title" id="user-config-title">Connection links</div>
      <textarea id="user-links" readonly></textarea>
      <div class="test-row">
        <button onclick="copyUserLinks()" class="primary">Copy</button>
        <button onclick="hideUserLinks()" class="ghost">Hide</button>
      </div>
      <div id="copy-msg" class="small"></div>
    </div>
    <div id="new-user-config" style="display:none; margin-top:12px">
      <div class="title">New connection links</div>
      <textarea id="new-user-links" readonly></textarea>
      <div class="test-row"><button onclick="copyNewUserLinks()" class="primary">Copy</button></div>
    </div>
  </div>
  <div class="card wide">
    <div class="title">SOCKS users</div>
    <div class="small">Each managed SOCKS user has a dedicated port. The legacy user stays on port 1080.</div>
    <div id="socks-list" class="row-list"></div>
    <div class="divider"></div>
    <div class="subhead">Add SOCKS user</div>
    <div class="form">
      <div><label>Username</label><input id="socks-user" placeholder="router2"></div>
      <div><label>Password</label><input id="socks-pass" placeholder="empty = generate"></div>
      <div><label>Port</label><input id="socks-port" placeholder="empty = next free"></div>
      <button onclick="addSocksUser()" class="primary">Add SOCKS user</button>
    </div>
    <div id="socks-msg" class="small"></div>
    <div id="socks-config" style="display:none; margin-top:12px">
      <div class="title" id="socks-config-title">SOCKS connection</div>
      <textarea id="socks-links" readonly></textarea>
      <div class="test-row">
        <button onclick="copySocksLinks()" class="primary">Copy</button>
        <button onclick="hideSocksLinks()" class="ghost">Hide</button>
      </div>
      <div id="socks-copy-msg" class="small"></div>
    </div>
  </div>
  <div class="card wide">
    <div class="title">Current Xray config</div>
    <div class="small">Read-only view of the active config file.</div>
    <div class="test-row"><button onclick="viewConfig()" class="primary">View config</button><button onclick="hideConfig()" class="ghost">Hide</button></div>
    <textarea id="config-view" readonly style="display:none; min-height:280px; margin-top:10px"></textarea>
    <div id="config-msg" class="small"></div>
  </div>
  <div class="card wide">
    <div class="title">Direct Iran domains</div>
    <div class="small">Domains added here use the direct outbound and the server IP.</div>
    <div id="domains-list" class="list"></div>
    <div class="form">
      <div><label>Domain</label><input id="direct-domain" placeholder="example.com"></div>
      <button onclick="addDirectDomain()" class="primary">Add direct domain</button>
    </div>
    <div id="domains-msg" class="small"></div>
  </div>`;
}
function testCls(status){ return status === 'ok' || status === 'redirect' ? 'ok' : (status === 'blocked' || status === 'timeout' || status === 'error' ? 'bad' : 'warn'); }
async function load(){
  try {
    const r = await fetch('api/status', {cache:'no-store'});
    const d = await r.json();
    meta.textContent = `updated: ${d.generated_at} | hostname: ${d.hostname}`;
    let h = '';
    h += statCard('CPU load', d.system.cpu.percent_label, d.system.cpu.detail, 'CPU', d.system.cpu.percent);
    h += statCard('RAM', d.system.ram.used_label, d.system.ram.detail, 'RAM', d.system.ram.percent);
    h += statCard('Inbound total', d.traffic.uplink_human, 'client to server', 'IN');
    h += statCard('Outbound total', d.traffic.downlink_human, 'server to client', 'OUT');
    h += statCard('Xray proxy tunnel', d.proxy_interface || 'route', 'current outbound interface', 'XR');
    h += statCard('tun0 traffic', `${d.tunnels.tun0.rx_human} / ${d.tunnels.tun0.tx_human}`, 'RX / TX since interface start', 'T0');
    h += statCard('tun1 traffic', `${d.tunnels.tun1.rx_human} / ${d.tunnels.tun1.tx_human}`, 'RX / TX since interface start', 'T1');
    for (const [name, value] of Object.entries(d.services)) h += statusCard(name, value);
    h += textCard('tun0', d.interfaces.tun0 || '-');
    h += textCard('tun1', d.interfaces.tun1 || '-');
    h += statusCard('Xray config', d.checks.xray_config.ok ? 'ok' : 'failed');
    h += healthCard('Proxy upstream', d.checks.proxy_upstream, 'UP');
    h += healthCard('Xray SOCKS path', d.checks.xray_socks, 'SO');
    h += routeCard(d.ai_route);
    h += testCard('socks_google', 'Google via Xray SOCKS');
    h += testCard('telegram_xray', 'Telegram via Xray SOCKS');
    h += testCard('direct_blubank', 'Blubank direct');
    h += testCard('tun1_chatgpt', 'ChatGPT via tun1');
    h += tunnelTestCard();
    h += preCard('Cloudflare routes', d.routes.cloudflare);
    h += preCard('Recent monitor logs', d.logs.monitor);
    statusGrid.innerHTML = h;
    if (activeTab === 'settings') await loadAdmin();
  } catch(e) {
    meta.textContent = 'error: ' + e;
  }
}
async function apiPost(action, payload){
  const r = await fetch(`api/xray/${action}`, {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify(payload || {})});
  const d = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(d.error || `HTTP ${r.status}`);
  if (!d.ok) throw new Error(d.error || 'failed');
  return d;
}
function sleep(ms){ return new Promise(resolve => setTimeout(resolve, ms)); }
async function safeLoadAdmin(){
  let last = null;
  for (const wait of [250, 700, 1200, 2000, 3500]) {
    try {
      await sleep(wait);
      await loadAdmin();
      return true;
    } catch(e) {
      last = e;
    }
  }
  throw last || new Error('Load failed');
}
async function loadAdmin(){
  const saved = {};
  for (const id of ['user-email','user-id','edit-user','edit-email','edit-id','direct-domain','socks-user','socks-pass','socks-port']) {
    const el = document.getElementById(id);
    if (el) saved[id] = el.value;
  }
  const r = await fetch('api/xray/config', {cache:'no-store'});
  if (!r.ok) throw new Error(`Load failed: HTTP ${r.status}`);
  const d = await r.json();
  const users = d.users || [];
  currentUsers = users;
  currentSocksUsers = d.socks_users || [];
  const domains = d.direct_domains || [];
  const usageMeta = d.usage_meta || {};
  document.getElementById('usage-meta').textContent = usageMeta.updated_human
    ? `Usage updated: ${usageMeta.updated_human} | tracked users: ${usageMeta.tracked_users || 0} | raw active users: ${usageMeta.raw_users || 0}`
    : 'Usage stats are not available.';
  document.getElementById('users-list').innerHTML = users.map(u => `
    <div class="row-item">
      <div class="row-main">
        <div class="row-title">${esc(u.email)}</div>
        <div class="small">${esc(u.id_masked)}</div>
        <div class="usage">
          <span class="small ${u.online ? 'ok' : 'warn'}">${u.online ? 'Online' : 'Offline'}</span>
          <span class="small">Sessions: ${esc(u.online_count ?? 0)}</span>
          <span class="small">${esc(u.online_source || 'api')}</span>
          <span class="small">Used: ${esc(u.usage_human || 'unknown')}</span>
          <span class="small">Limit: ${esc(u.limit_human || 'none')}</span>
          <span class="small">Speed: ${esc(u.bandwidth_human || 'none')}</span>
          <span class="small ${u.quota_status === 'over' ? 'bad' : 'ok'}">${esc(u.quota_status || 'unlimited')}</span>
        </div>
        <div class="small">${u.online_ips && u.online_ips.length ? `IPs: ${esc(u.online_ips.join(', '))}` : ''}</div>
      </div>
      <div class="row-actions">
        <button onclick="openActivityModal('${esc(u.email)}')">Activity</button>
        <button onclick="showUserConfig('${esc(u.email)}')">Config</button>
        <button onclick="setUserLimit('${esc(u.email)}')">Limit</button>
        <button onclick="setBandwidthLimit('${esc(u.email)}')">Speed</button>
        <button onclick="tempBanUser('${esc(u.email)}')">Temp ban</button>
        <button onclick="startEditUser('${esc(u.email)}')">Edit</button>
        <button class="danger" onclick="deleteUser('${esc(u.email)}')">Delete</button>
      </div>
    </div>`).join('') || '<span class="small">No users</span>';
  const banned = d.banned_users || [];
  if (banned.length) {
    document.getElementById('users-list').innerHTML += banned.map(u => `
      <div class="row-item">
        <div class="row-main">
          <div class="row-title">${esc(u.email)}</div>
          <div class="small">Temporarily banned until ${esc(u.until_human)}</div>
        </div>
        <div class="row-actions">
          <button onclick="showBannedConfig('${esc(u.email)}')">Config</button>
          <button class="primary" onclick="unbanUser('${esc(u.email)}')">Unban now</button>
        </div>
      </div>`).join('');
  }
  document.getElementById('socks-list').innerHTML = currentSocksUsers.map(u => `
    <div class="row-item">
      <div class="row-main">
        <div class="row-title">${esc(u.user)} <span class="small">:${esc(u.port)}</span></div>
        <div class="small">${esc(u.host)}:${esc(u.port)} | ${esc(u.tag || '')}</div>
        <div class="usage">
          <span class="small ${u.online ? 'ok' : 'warn'}">${u.online ? 'Online' : 'Offline'}</span>
          <span class="small">Sessions: ${esc(u.online_count ?? 0)}</span>
          <span class="small">Used: ${esc(u.usage_human || 'unknown')}</span>
          <span class="small">Upload: ${esc(u.upload_human || '0 B')}</span>
          <span class="small">Download: ${esc(u.download_human || '0 B')}</span>
        </div>
        <div class="small">${u.online_ips && u.online_ips.length ? `IPs: ${esc(u.online_ips.join(', '))}` : ''}</div>
      </div>
      <div class="row-actions">
        <button onclick="showSocksConfig('${esc(u.user)}')">Config</button>
        <button onclick="editSocksUser('${esc(u.user)}')">Edit</button>
        <button class="danger" onclick="deleteSocksUser('${esc(u.user)}')">Delete</button>
      </div>
    </div>`).join('') || '<span class="small">No managed SOCKS users</span>';
  document.getElementById('domains-list').innerHTML = domains.map(d => `<span class="pill">${esc(d)} <button class="danger" onclick="deleteDirectDomain('${esc(d)}')">Remove</button></span>`).join('') || '<span class="small">no direct domains</span>';
  for (const [id, value] of Object.entries(saved)) {
    const el = document.getElementById(id);
    if (el) el.value = value;
  }
}
async function openActivityModal(email){
  currentActivityEmail = email;
  document.getElementById('activity-title').textContent = `Activity for ${email}`;
  document.getElementById('activity-modal').style.display = 'flex';
  await refreshActivity();
}
function closeActivityModal(){
  document.getElementById('activity-modal').style.display = 'none';
}
async function refreshActivity(){
  const list = document.getElementById('activity-list');
  const subtitle = document.getElementById('activity-subtitle');
  if (!currentActivityEmail) return;
  list.innerHTML = '<div class="small">Loading...</div>';
  try {
    const r = await fetch(`api/xray/activity?email=${encodeURIComponent(currentActivityEmail)}&seconds=60`, {cache:'no-store'});
    const d = await r.json();
    subtitle.textContent = `Last ${d.seconds}s | generated ${d.generated_at}`;
    const items = d.items || [];
    list.innerHTML = items.length ? items.map(a => `
      <div class="small activity-line">
        <span class="activity-time">${esc(a.time)}</span>
        <span class="activity-dest">${esc(a.destination)}</span>
        <span class="activity-tag">${esc(a.outbound || '')}</span>
        <span>${esc(a.client_ip || '')}</span>
      </div>`).join('') : '<div class="small">No activity in the selected window.</div>';
  } catch(e) {
    list.innerHTML = `<div class="small danger">Error: ${esc(e.message)}</div>`;
  }
}
async function viewConfig(){
  const msg = document.getElementById('config-msg');
  const area = document.getElementById('config-view');
  try {
    const r = await fetch('api/xray/raw-config', {cache:'no-store'});
    const text = await r.text();
    if (!r.ok) throw new Error(text);
    area.value = text;
    area.style.display = 'block';
    msg.textContent = 'Loaded active config.';
  } catch(e) { msg.textContent = `Error: ${e.message}`; }
}
function hideConfig(){
  document.getElementById('config-view').style.display = 'none';
}
async function setUserLimit(email){
  const current = currentUsers.find(u => u.email === email);
  const existing = current && current.limit_gb ? current.limit_gb : '';
  const input = prompt(`Traffic limit for ${email} in GB. Empty = unlimited.`, existing);
  if (input === null) return;
  const msg = document.getElementById('users-msg');
  try {
    const limit_gb = input.trim();
    await apiPost('set-user-limit', {email, limit_gb});
    msg.textContent = limit_gb ? `Set ${email} limit to ${limit_gb} GB` : `Removed ${email} limit`;
    await safeLoadAdmin();
  } catch(e) { msg.textContent = `Error: ${e.message}`; }
}
async function setBandwidthLimit(email){
  const current = currentUsers.find(u => u.email === email);
  const existing = current && current.bandwidth ? current.bandwidth : {};
  const down = prompt(`Download speed for ${email} in Mbps. Empty = no download cap. Empty both prompts removes the limited inbound.`, existing.download_mbps || '');
  if (down === null) return;
  const up = prompt(`Upload speed for ${email} in Mbps. Empty = no upload cap.`, existing.upload_mbps || '');
  if (up === null) return;
  const msg = document.getElementById('users-msg');
  try {
    const d = await apiPost('set-bandwidth-limit', {email, download_mbps: down.trim(), upload_mbps: up.trim()});
    if (d.bandwidth) {
      msg.textContent = `Set ${email} speed limit. Use the limited connection link for enforcement.`;
      showNewUserLinks((d.links || []).filter(link => String(link.name || '').includes('limited')));
    } else {
      msg.textContent = `Removed ${email} speed limit.`;
    }
    await safeLoadAdmin();
  } catch(e) { msg.textContent = `Error: ${e.message}`; }
}
async function tempBanUser(email){
  const input = prompt(`Temporary ban duration for ${email} in minutes.`, '60');
  if (input === null) return;
  const msg = document.getElementById('users-msg');
  try {
    const minutes = input.trim();
    const d = await apiPost('temp-ban-user', {email, minutes});
    msg.textContent = `Banned ${email} until ${d.until_human}`;
    await safeLoadAdmin();
  } catch(e) { msg.textContent = `Error: ${e.message}`; }
}
async function unbanUser(email){
  const msg = document.getElementById('users-msg');
  try {
    const d = await apiPost('unban-user', {email});
    msg.textContent = `Unbanned ${d.email}`;
    await safeLoadAdmin();
  } catch(e) { msg.textContent = `Error: ${e.message}`; }
}
function startEditUser(email){
  const user = currentUsers.find(u => u.email === email);
  document.getElementById('edit-panel').style.display = 'block';
  document.getElementById('edit-user').value = email;
  document.getElementById('edit-email').value = email;
  document.getElementById('edit-id').value = '';
  document.getElementById('users-msg').textContent = user ? `Editing ${email}` : '';
  document.getElementById('edit-email').focus();
}
function formatLinks(links){
  return (links || []).map(item => `${item.name}\\n${item.url}`).join('\\n\\n');
}
function showUserConfig(email){
  const user = currentUsers.find(u => u.email === email);
  const box = document.getElementById('user-config');
  const area = document.getElementById('user-links');
  const title = document.getElementById('user-config-title');
  if (!user || !user.links || !user.links.length) {
    document.getElementById('users-msg').textContent = `No connection links for ${email}`;
    return;
  }
  title.textContent = `Connection links for ${email}`;
  area.value = formatLinks(user.links);
  box.style.display = 'block';
  area.focus();
  area.select();
}
async function showBannedConfig(email){
  try {
    const r = await fetch('api/xray/config', {cache:'no-store'});
    const d = await r.json();
    const user = (d.banned_users || []).find(u => u.email === email);
    if (!user || !user.links || !user.links.length) {
      document.getElementById('users-msg').textContent = `No connection links for ${email}`;
      return;
    }
    const box = document.getElementById('user-config');
    const area = document.getElementById('user-links');
    document.getElementById('user-config-title').textContent = `Connection links for ${email}`;
    area.value = formatLinks(user.links);
    box.style.display = 'block';
    area.focus();
    area.select();
  } catch(e) {
    document.getElementById('users-msg').textContent = `Error: ${e.message}`;
  }
}
function hideUserLinks(){
  document.getElementById('user-config').style.display = 'none';
  document.getElementById('user-links').value = '';
  document.getElementById('copy-msg').textContent = '';
}
async function copyTextFrom(id, msgId){
  const text = document.getElementById(id).value;
  const msg = document.getElementById(msgId);
  try {
    await navigator.clipboard.writeText(text);
    msg.textContent = 'Copied.';
  } catch(e) {
    document.getElementById(id).focus();
    document.getElementById(id).select();
    msg.textContent = 'Select and copy manually.';
  }
}
function copyUserLinks(){ copyTextFrom('user-links', 'copy-msg'); }
function copyNewUserLinks(){ copyTextFrom('new-user-links', 'users-msg'); }
function copySocksLinks(){ copyTextFrom('socks-links', 'socks-copy-msg'); }
function showSocksConfig(user){
  const item = currentSocksUsers.find(u => u.user === user);
  if (!item) return;
  document.getElementById('socks-config-title').textContent = `SOCKS connection for ${user}`;
  document.getElementById('socks-links').value = formatLinks(item.links || []);
  document.getElementById('socks-config').style.display = 'block';
  document.getElementById('socks-links').focus();
  document.getElementById('socks-links').select();
}
function hideSocksLinks(){
  document.getElementById('socks-config').style.display = 'none';
  document.getElementById('socks-links').value = '';
  document.getElementById('socks-copy-msg').textContent = '';
}
async function addSocksUser(){
  const msg = document.getElementById('socks-msg');
  try {
    const user = document.getElementById('socks-user').value.trim();
    const password = document.getElementById('socks-pass').value.trim();
    const port = document.getElementById('socks-port').value.trim();
    const d = await apiPost('add-socks-user', {user, password, port});
    msg.textContent = `Added SOCKS user ${d.user} on port ${d.port}`;
    document.getElementById('socks-user').value = '';
    document.getElementById('socks-pass').value = '';
    document.getElementById('socks-port').value = '';
    await safeLoadAdmin();
    const item = (currentSocksUsers || []).find(u => u.user === d.user);
    if (item) showSocksConfig(d.user);
  } catch(e) { msg.textContent = `Error: ${e.message}`; }
}
async function editSocksUser(user){
  const current = currentSocksUsers.find(u => u.user === user);
  if (!current) return;
  const nextUser = prompt(`Username for ${user}`, current.user);
  if (nextUser === null) return;
  const nextPass = prompt(`Password for ${user}. Leave empty to keep current password.`, '');
  if (nextPass === null) return;
  const nextPort = prompt(`Port for ${user}`, String(current.port));
  if (nextPort === null) return;
  const msg = document.getElementById('socks-msg');
  try {
    const d = await apiPost('edit-socks-user', {old_user: user, user: nextUser.trim(), password: nextPass.trim(), port: nextPort.trim()});
    msg.textContent = `Edited SOCKS user ${d.user}`;
    await safeLoadAdmin();
    showSocksConfig(d.user);
  } catch(e) { msg.textContent = `Error: ${e.message}`; }
}
async function deleteSocksUser(user){
  const msg = document.getElementById('socks-msg');
  try {
    if (!confirm(`Delete SOCKS user ${user}?`)) return;
    await apiPost('delete-socks-user', {user});
    msg.textContent = `Deleted SOCKS user ${user}`;
    hideSocksLinks();
    await safeLoadAdmin();
  } catch(e) { msg.textContent = `Error: ${e.message}`; }
}
function cancelEdit(){
  document.getElementById('edit-panel').style.display = 'none';
  document.getElementById('edit-user').value = '';
  document.getElementById('edit-email').value = '';
  document.getElementById('edit-id').value = '';
  document.getElementById('users-msg').textContent = '';
}
async function addUser(){
  const msg = document.getElementById('users-msg');
  try {
    const email = document.getElementById('user-email').value.trim();
    const id = document.getElementById('user-id').value.trim();
    const d = await apiPost('add-user', {email, id});
    msg.textContent = `Added ${d.email}`;
    showNewUserLinks(d.links || []);
    document.getElementById('user-email').value = '';
    document.getElementById('user-id').value = '';
    await safeLoadAdmin();
  } catch(e) { msg.textContent = `Error: ${e.message}`; }
}
function showNewUserLinks(links){
  const box = document.getElementById('new-user-config');
  const area = document.getElementById('new-user-links');
  if (!links.length) {
    box.style.display = 'none';
    area.value = '';
    return;
  }
  area.value = formatLinks(links);
  box.style.display = 'block';
  area.focus();
  area.select();
}
async function editUser(){
  const msg = document.getElementById('users-msg');
  try {
    const old_email = document.getElementById('edit-user').value;
    const email = document.getElementById('edit-email').value.trim();
    const id = document.getElementById('edit-id').value.trim();
    const d = await apiPost('edit-user', {old_email, email, id});
    msg.textContent = `Edited ${d.email}`;
    cancelEdit();
    document.getElementById('edit-email').value = '';
    document.getElementById('edit-id').value = '';
    await safeLoadAdmin();
  } catch(e) { msg.textContent = `Error: ${e.message}`; }
}
async function deleteUser(email){
  const msg = document.getElementById('users-msg');
  try {
    if (!email || !confirm(`Delete ${email}?`)) return;
    await apiPost('delete-user', {email});
    msg.textContent = `Deleted ${email}`;
    await safeLoadAdmin();
  } catch(e) { msg.textContent = `Error: ${e.message}`; }
}
async function addDirectDomain(){
  const msg = document.getElementById('domains-msg');
  try {
    const domain = document.getElementById('direct-domain').value.trim();
    const d = await apiPost('add-direct-domain', {domain});
    msg.textContent = `Added ${d.domain}`;
    document.getElementById('direct-domain').value = '';
    await safeLoadAdmin();
  } catch(e) { msg.textContent = `Error: ${e.message}`; }
}
async function deleteDirectDomain(domain){
  const msg = document.getElementById('domains-msg');
  try {
    if (!confirm(`Remove ${domain}?`)) return;
    await apiPost('delete-direct-domain', {domain});
    msg.textContent = `Removed ${domain}`;
    await safeLoadAdmin();
  } catch(e) { msg.textContent = `Error: ${e.message}`; }
}
async function runTest(key){
  const value = document.getElementById(`test-${key}`);
  const detail = document.getElementById(`test-${key}-detail`);
  value.className = 'value warn';
  value.textContent = '⏳ Testing...';
  detail.textContent = '';
  try {
    const r = await fetch(`api/test?name=${encodeURIComponent(key)}`, {cache:'no-store'});
    const d = await r.json();
    const c = testCls(d.status);
    value.className = `value ${c}`;
    value.textContent = `${emoji(c === 'ok' ? 'ok' : (c === 'bad' ? 'failed' : 'warn'))} ${d.label}`;
    detail.textContent = d.detail || '';
  } catch(e) {
    value.className = 'value bad';
    value.textContent = '❌ Test failed';
    detail.textContent = String(e);
  }
}
async function runTunnelTargetTest(){
  const result = document.getElementById('tunnel-test-result');
  const target = document.getElementById('tunnel-target').value.trim();
  const port = document.getElementById('tunnel-port').value.trim();
  const iface = document.getElementById('tunnel-interface').value;
  result.className = 'small warn';
  result.textContent = 'Testing...';
  try {
    const qs = new URLSearchParams({target, port, interface: iface});
    const r = await fetch(`api/tunnel-test?${qs.toString()}`, {cache:'no-store'});
    const d = await r.json();
    const c = testCls(d.status);
    result.className = `small ${c}`;
    result.textContent = `${d.label} | ${d.detail || ''}`;
  } catch(e) {
    result.className = 'small bad';
    result.textContent = `Error: ${e.message}`;
  }
}
document.getElementById('refresh').onclick = load;
load(); setInterval(load, 15000);
</script>
</body>
</html>"""


def run(cmd, timeout=6):
    try:
        p = subprocess.run(cmd, shell=True, text=True, capture_output=True, timeout=timeout)
        return {"rc": p.returncode, "out": p.stdout.strip(), "err": p.stderr.strip()}
    except subprocess.TimeoutExpired:
        return {"rc": 124, "out": "", "err": "timeout"}


def run_async(cmd):
    subprocess.Popen(["/bin/bash", "-lc", cmd], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def service(name):
    return run(f"systemctl is-active {name}", 3)["out"] or "unknown"


def intentionally_disabled_service(name):
    state = service(name)
    return "disabled" if state in ("inactive", "failed", "unknown") else state


def service_statuses(names, disabled_names=()):
    result = {name: "unknown" for name in names}
    quoted = " ".join(shlex.quote(name) for name in names)
    r = run(f"systemctl is-active {quoted}", 4)
    lines = (r["out"] or "").splitlines()
    for name, state in zip(names, lines):
        result[name] = state or "unknown"
    for name in disabled_names:
        if result.get(name) in ("inactive", "failed", "unknown"):
            result[name] = "disabled"
    return result


def iface(name):
    return run(f"ip -br addr show dev {name} 2>/dev/null", 3)["out"]


def tunnel_counters(name):
    base = f"/sys/class/net/{name}/statistics"
    try:
        rx = int(open(os.path.join(base, "rx_bytes")).read().strip())
        tx = int(open(os.path.join(base, "tx_bytes")).read().strip())
        rx_packets = int(open(os.path.join(base, "rx_packets")).read().strip())
        tx_packets = int(open(os.path.join(base, "tx_packets")).read().strip())
    except Exception:
        rx = tx = rx_packets = tx_packets = 0
    return {
        "rx_bytes": rx,
        "tx_bytes": tx,
        "rx_human": human_bytes(rx),
        "tx_human": human_bytes(tx),
        "rx_packets": rx_packets,
        "tx_packets": tx_packets,
    }


def proxy_interface():
    try:
        cfg = load_xray_config()
        for outbound in cfg.get("outbounds", []):
            if outbound.get("tag") == "proxy":
                return outbound.get("streamSettings", {}).get("sockopt", {}).get("interface", "") or "route"
    except Exception:
        pass
    return "unknown"


AI_ROUTE_DOMAINS = (
    "domain:chatgpt.com",
    "domain:openai.com",
    "domain:claude.ai",
    "domain:anthropic.com",
    "domain:cursor.com",
)


def iface_ipv4_addr(name):
    r = run(
        f"ip -4 -o addr show dev {shlex.quote(name)} scope global | awk '{{split($4,a,\"/\"); print a[1]; exit}}'",
        3,
    )
    return (r.get("out") or "").strip()


def iface_has_ipv4(name):
    return bool(iface_ipv4_addr(name))


def tunnel_edge_health(name):
    src = iface_ipv4_addr(name)
    if not src:
        return False
    r = run(
        f"timeout 4 nc -z -w 3 -s {shlex.quote(src)} {shlex.quote(EDGE_UPSTREAM_IP)} 443",
        5,
    )
    return r["rc"] == 0


def ai_route_status():
    tun0_up = iface_has_ipv4("tun0")
    tun1_up = iface_has_ipv4("tun1")
    tun0_ok = tunnel_edge_health("tun0")
    tun1_ok = tunnel_edge_health("tun1")
    desired = "proxy" if (tun0_ok or tun1_ok) else "priority-proxy"
    desired_if = "tun0" if tun0_ok else ("tun1" if tun1_ok else "")
    current = "missing"
    current_if = ""
    try:
        cfg = load_xray_config()
        for rule in cfg.get("routing", {}).get("rules", []):
            domains = rule.get("domain") or []
            if any(domain in domains for domain in AI_ROUTE_DOMAINS):
                current = rule.get("outboundTag") or "missing"
                break
        for outbound in cfg.get("outbounds", []):
            if outbound.get("tag") == "proxy":
                current_if = outbound.get("streamSettings", {}).get("sockopt", {}).get("interface", "")
                break
    except Exception as exc:
        return {
            "ok": False,
            "state": "error",
            "summary": f"Could not read Xray config: {exc}",
        }
    watcher = service("xray-ai-route-failover")
    ok = current == desired and watcher == "active" and (desired != "proxy" or current_if == desired_if)
    tunnel_text = f"tun0={'ok' if tun0_ok else ('up' if tun0_up else 'down')}, tun1={'ok' if tun1_ok else ('up' if tun1_up else 'down')}"
    return {
        "ok": ok,
        "state": "ok" if ok else "mismatch",
        "current": current,
        "desired": desired,
        "current_if": current_if,
        "desired_if": desired_if,
        "watcher": watcher,
        "summary": f"current={current}/{current_if or '-'} desired={desired}/{desired_if or '-'} watcher={watcher} {tunnel_text}",
    }


def curl_summary(cmd):
    r = run(cmd, 12)
    out = (r["out"] or "").splitlines()[-1] if r["out"] else ""
    if not out and r["err"]:
        out = "err=" + r["err"].splitlines()[-1]
    return {"ok": r["rc"] == 0, "summary": out or f"rc={r['rc']}"}


def human_http_result(result):
    summary = result.get("summary", "")
    fields = {}
    parts = summary.split("|") if "|" in summary else summary.split()
    for part in parts:
        if "=" in part:
            key, value = part.split("=", 1)
            fields[key] = value
    code = fields.get("http", "000")
    err = fields.get("err", "")
    ip = fields.get("ip", "")
    elapsed = fields.get("time", "")
    if code in ("200", "204"):
        status, label = "ok", "Reachable"
    elif code in ("301", "302", "303", "307", "308"):
        status, label = "redirect", "Reachable - redirect"
    elif code in ("401", "403", "451"):
        status, label = "blocked", f"Blocked or restricted ({code})"
    elif code == "000" and ("timeout" in err.lower() or "timed out" in err.lower()):
        status, label = "timeout", "Timeout"
    elif code == "000":
        status, label = "error", "Connection error"
    elif code.startswith("4"):
        status, label = "ok", f"Reachable - HTTP {code}"
    elif code.startswith("5"):
        status, label = "warn", f"Server response ({code})"
    else:
        status, label = "warn", f"HTTP {code}"
    bits = []
    if ip:
        bits.append("via=local proxy" if ip.startswith("127.") else f"ip={ip}")
    if elapsed:
        bits.append(f"time={elapsed}s")
    if err:
        bits.append(f"err={err}")
    return {"status": status, "label": label, "detail": " ".join(bits)}


def socks_creds():
    try:
        with open("/usr/local/etc/xray/config.json") as f:
            cfg = json.load(f)
        for inbound in cfg.get("inbounds", []):
            if inbound.get("protocol") == "socks" and inbound.get("port") == 1080:
                acct = inbound.get("settings", {}).get("accounts", [{}])[0]
                return acct.get("user", ""), acct.get("pass", "")
    except Exception:
        pass
    return "", ""


def load_xray_config():
    with open("/usr/local/etc/xray/config.json") as f:
        return json.load(f)


def load_quotas():
    try:
        with open(QUOTA_FILE) as f:
            data = json.load(f)
        return data if isinstance(data, dict) else {}
    except Exception:
        return {}


def save_quotas(data):
    tmp = QUOTA_FILE + ".tmp"
    with open(tmp, "w") as f:
        json.dump(data, f, indent=2, sort_keys=True)
        f.write("\n")
    os.replace(tmp, QUOTA_FILE)


def load_bans():
    try:
        with open(BAN_FILE) as f:
            data = json.load(f)
        return data if isinstance(data, dict) else {}
    except Exception:
        return {}


def save_bans(data):
    tmp = BAN_FILE + ".tmp"
    with open(tmp, "w") as f:
        json.dump(data, f, indent=2, sort_keys=True)
        f.write("\n")
    os.replace(tmp, BAN_FILE)


def load_bandwidth_limits():
    try:
        with open(BANDWIDTH_FILE) as f:
            data = json.load(f)
        return data if isinstance(data, dict) else {}
    except Exception:
        return {}


def save_bandwidth_limits(data):
    tmp = BANDWIDTH_FILE + ".tmp"
    with open(tmp, "w") as f:
        json.dump(data, f, indent=2, sort_keys=True)
        f.write("\n")
    os.replace(tmp, BANDWIDTH_FILE)


def load_usage_state():
    try:
        with open(USAGE_FILE) as f:
            data = json.load(f)
        if isinstance(data, dict):
            data.setdefault("totals", {})
            data.setdefault("last_raw", {})
            return data
    except Exception:
        pass
    return {"totals": {}, "last_raw": {}}


def save_usage_state(data):
    tmp = USAGE_FILE + ".tmp"
    with open(tmp, "w") as f:
        json.dump(data, f, indent=2, sort_keys=True)
        f.write("\n")
    os.replace(tmp, USAGE_FILE)


def rename_user_state(old_email, new_email):
    if old_email == new_email:
        return
    with STATE_LOCK:
        quotas = load_quotas()
        if old_email in quotas:
            quotas[new_email] = quotas.pop(old_email)
            save_quotas(quotas)
        state = load_usage_state()
        changed = False
        for bucket in ("totals", "last_raw"):
            values = state.setdefault(bucket, {})
            if old_email in values:
                current = values.pop(old_email)
                existing = values.get(new_email)
                if isinstance(current, dict) and isinstance(existing, dict):
                    merged = {}
                    for direction in ("uplink", "downlink"):
                        merged[direction] = int(existing.get(direction, 0) or 0) + int(current.get(direction, 0) or 0)
                    values[new_email] = merged
                else:
                    values[new_email] = current
                changed = True
        if changed:
            save_usage_state(state)


def delete_user_state(email):
    with STATE_LOCK:
        quotas = load_quotas()
        if email in quotas:
            quotas.pop(email, None)
            save_quotas(quotas)
        state = load_usage_state()
        changed = False
        for bucket in ("totals", "last_raw"):
            values = state.setdefault(bucket, {})
            if email in values:
                values.pop(email, None)
                changed = True
        if changed:
            save_usage_state(state)


def find_vless_user(cfg, email):
    for inbound in cfg.get("inbounds", []):
        if inbound.get("protocol") != "vless":
            continue
        for client in inbound.get("settings", {}).get("clients", []):
            if client.get("email") == email:
                return {"id": client.get("id", ""), "email": email}
    return None


def is_limited_inbound(inbound):
    return str(inbound.get("tag") or "").startswith("limited-")


def inbound_has_client(inbound, email, user_id=None):
    for client in inbound.get("settings", {}).get("clients", []):
        if client.get("email") == email and (not user_id or client.get("id") == user_id):
            return True
    return False


def remove_vless_user(cfg, email):
    touched = 0
    for inbound in cfg.get("inbounds", []):
        if inbound.get("protocol") != "vless":
            continue
        clients = inbound.get("settings", {}).get("clients", [])
        before = len(clients)
        inbound["settings"]["clients"] = [c for c in clients if c.get("email") != email]
        touched += before - len(inbound["settings"]["clients"])
    return touched


def add_vless_user(cfg, email, user_id):
    touched = 0
    for inbound in cfg.get("inbounds", []):
        if inbound.get("protocol") != "vless":
            continue
        if is_limited_inbound(inbound):
            continue
        clients = inbound.get("settings", {}).setdefault("clients", [])
        if not any(c.get("email") == email for c in clients):
            clients.append({"id": user_id, "email": email})
            touched += 1
    return touched


def human_bytes(value):
    try:
        value = float(value)
    except Exception:
        return "unknown"
    units = ["B", "KB", "MB", "GB", "TB"]
    idx = 0
    while value >= 1024 and idx < len(units) - 1:
        value /= 1024
        idx += 1
    return f"{value:.1f} {units[idx]}"


def read_cpu_times():
    with open("/proc/stat") as f:
        parts = [int(x) for x in f.readline().split()[1:8]]
    idle = parts[3] + parts[4]
    total = sum(parts)
    return total, idle


def cpu_metrics():
    global CPU_LAST
    cpu = {"percent": 0, "percent_label": "0%", "detail": "cpu unavailable"}
    try:
        with CPU_LOCK:
            first = CPU_LAST or read_cpu_times()
            if CPU_LAST is None:
                time.sleep(0.2)
            second = read_cpu_times()
            CPU_LAST = second
        total_delta = second[0] - first[0]
        idle_delta = second[1] - first[1]
        with open("/proc/loadavg") as f:
            load1, load5, load15 = [float(x) for x in f.read().split()[:3]]
        cores = os.cpu_count() or 1
        raw_percent = 0 if total_delta <= 0 else max(0.0, min(100.0, (total_delta - idle_delta) / total_delta * 100))
        load_percent = max(0.0, min(100.0, (load1 / cores) * 100))
        # /api/status can be hit concurrently with shell checks; one short /proc/stat
        # sample then reports a misleading 100%. Keep the card tied to sustained load.
        percent = load_percent if raw_percent > 90 and load_percent < 80 else raw_percent
        cpu = {
            "percent": round(percent, 1),
            "percent_label": f"{percent:.0f}%",
            "detail": f"load {load1:.2f}, {load5:.2f}, {load15:.2f} on {cores} core",
        }
    except Exception:
        pass
    return cpu


def system_metrics():
    cpu = cpu_metrics()
    ram = {"percent": 0, "used_label": "unknown", "detail": "memory unavailable"}
    try:
        values = {}
        with open("/proc/meminfo") as f:
            for line in f:
                key, rest = line.split(":", 1)
                values[key] = int(rest.strip().split()[0]) * 1024
        total = values.get("MemTotal", 0)
        available = values.get("MemAvailable", 0)
        used = max(0, total - available)
        percent = (used / total * 100) if total else 0
        ram = {
            "percent": round(percent, 1),
            "used_bytes": used,
            "total_bytes": total,
            "used_label": f"{percent:.0f}%",
            "detail": f"{human_bytes(used)} used of {human_bytes(total)}",
        }
    except Exception:
        pass
    return {"cpu": cpu, "ram": ram}


def traffic_totals():
    now = time.time()
    cached = STATUS_CACHE["traffic"]
    if cached["value"] is not None and now - cached["ts"] < 15:
        return cached["value"]
    usage = persistent_usage_totals()
    uplink = sum(int(v.get("uplink", 0) or 0) for v in usage.values())
    downlink = sum(int(v.get("downlink", 0) or 0) for v in usage.values())
    value = {
        "uplink_bytes": uplink,
        "downlink_bytes": downlink,
        "uplink_human": human_bytes(uplink),
        "downlink_human": human_bytes(downlink),
        "users": len(usage),
    }
    STATUS_CACHE["traffic"] = {"ts": now, "value": value}
    return value


def cached_xray_config_ok():
    path = "/usr/local/etc/xray/config.json"
    now = time.time()
    try:
        mtime = os.path.getmtime(path)
    except Exception:
        mtime = 0
    cached = STATUS_CACHE["xray_config"]
    if cached["mtime"] == mtime and now - cached["ts"] < 60:
        return bool(cached["ok"])
    ok = run("xray run -test -config /usr/local/etc/xray/config.json >/dev/null 2>&1", 8)["rc"] == 0
    STATUS_CACHE["xray_config"] = {"mtime": mtime, "ts": now, "ok": ok}
    return ok


def curl_health(cmd, timeout=10):
    result = curl_summary(cmd)
    parsed = human_http_result(result)
    return {
        "ok": parsed.get("status") in ("ok", "redirect", "warn", "blocked"),
        "status": parsed.get("status"),
        "label": parsed.get("label"),
        "detail": parsed.get("detail") or result.get("summary", ""),
    }


def proxy_upstream_health():
    now = time.time()
    cached = STATUS_CACHE["proxy_upstream"]
    if cached["value"] is not None and now - cached["ts"] < 60:
        return cached["value"]
    try:
        cfg = load_xray_config()
        outbound = next(out for out in cfg.get("outbounds", []) if out.get("tag") == "proxy")
        vnext = outbound.get("settings", {}).get("vnext", [{}])[0]
        address = vnext.get("address")
        port = int(vnext.get("port", 443))
        stream = outbound.get("streamSettings", {})
        interface = stream.get("sockopt", {}).get("interface", "")
        src = iface_ipv4_addr(interface) if interface else ""
        if interface and not src:
            value = {"ok": False, "status": "down", "label": "Interface down", "detail": f"{interface} has no IPv4"}
        else:
            src_arg = f"-s {shlex.quote(src)} " if src else ""
            started = time.time()
            r = run(f"timeout 5 nc -z -w 4 {src_arg}{shlex.quote(str(address))} {port}", 6)
            elapsed = time.time() - started
            if r["rc"] == 0:
                value = {"ok": True, "status": "ok", "label": "Reachable", "detail": f"tcp=open time={elapsed:.3f}s"}
            else:
                err = (r.get("err") or r.get("out") or "tcp connect failed").strip()
                value = {"ok": False, "status": "failed", "label": "Failed", "detail": f"tcp=failed time={elapsed:.3f}s err={err}"}
        value["detail"] = f"{address}:{port} via {interface or 'route'} | {value['detail']}"
    except Exception as exc:
        value = {"ok": False, "status": "error", "label": "Error", "detail": str(exc)}
    STATUS_CACHE["proxy_upstream"] = {"ts": now, "value": value}
    return value


def xray_socks_health():
    now = time.time()
    cached = STATUS_CACHE["xray_socks"]
    if cached["value"] is not None and now - cached["ts"] < 60:
        return cached["value"]
    user, password = socks_creds()
    proxy = (
        f"--socks5-hostname {shlex.quote(user)}:{shlex.quote(password)}@127.0.0.1:1080"
        if user and password
        else "--socks5-hostname 127.0.0.1:1080"
    )
    cmd = (
        f"curl -k --connect-timeout 6 --max-time 10 -sS -o /dev/null "
        f"-w 'http=%{{http_code}}|ip=%{{remote_ip}}|time=%{{time_total}}|err=%{{errormsg}}' "
        f"{proxy} https://telegram.org/"
    )
    value = curl_health(cmd)
    STATUS_CACHE["xray_socks"] = {"ts": now, "value": value}
    return value


def stats_enabled(cfg):
    return cfg.get("stats") == {} and "api" in cfg


def ensure_stats_config(cfg):
    changed = False
    if cfg.get("stats") != {}:
        cfg["stats"] = {}
        changed = True
    api = cfg.setdefault("api", {})
    if api.get("tag") != "api" or "StatsService" not in api.get("services", []):
        api["tag"] = "api"
        services = api.setdefault("services", [])
        if "StatsService" not in services:
            services.append("StatsService")
        changed = True
    policy = cfg.setdefault("policy", {})
    levels = policy.setdefault("levels", {})
    level0 = levels.setdefault("0", {})
    if level0.get("statsUserUplink") is not True or level0.get("statsUserDownlink") is not True:
        level0["statsUserUplink"] = True
        level0["statsUserDownlink"] = True
        changed = True
    inbounds = cfg.setdefault("inbounds", [])
    if not any(i.get("tag") == "xray-api" for i in inbounds):
        inbounds.append({
            "tag": "xray-api",
            "listen": "127.0.0.1",
            "port": 10085,
            "protocol": "dokodemo-door",
            "settings": {"address": "127.0.0.1"}
        })
        changed = True
    routing = cfg.setdefault("routing", {})
    rules = routing.setdefault("rules", [])
    if not any(r.get("inboundTag") == ["xray-api"] and r.get("outboundTag") == "api" for r in rules):
        rules.insert(0, {"type": "field", "inboundTag": ["xray-api"], "outboundTag": "api"})
        changed = True
    return changed


def query_user_usage():
    result = {}
    r = run(f"xray api statsquery --server={XRAY_API_SERVER} -pattern 'user>>>'", 5)
    if r["rc"] != 0:
        return result
    try:
        data = json.loads(r["out"])
    except Exception:
        return result
    for stat in data.get("stat", []) + data.get("stats", []):
        name = stat.get("name", "")
        value = int(stat.get("value", 0) or 0)
        parts = name.split(">>>")
        if len(parts) >= 4 and parts[0] == "user":
            email = parts[1]
            if email in INTERNAL_STATS_USERS:
                continue
            direction = parts[-1]
            item = result.setdefault(email, {"uplink": 0, "downlink": 0})
            if direction in item:
                item[direction] += value
    return result


def query_online_user(email):
    count = 0
    ips = []
    source = "api"
    r = run(f"xray api statsonline --server={XRAY_API_SERVER} -email {email}", 4)
    if r["rc"] == 0:
        try:
            data = json.loads(r["out"] or "{}")
            count = int(data.get("count", data.get("value", 0)) or 0)
        except Exception:
            numbers = re.findall(r"\d+", r["out"] or "")
            count = int(numbers[-1]) if numbers else 0
    elif "not found" in (r["err"] + r["out"]).lower():
        count = 0
    ipr = run(f"xray api statsonlineiplist --server={XRAY_API_SERVER} -email {email}", 4)
    if ipr["rc"] == 0:
        text = ipr["out"] or ""
        try:
            data = json.loads(text or "{}")
            for key in ("ips", "ip", "items", "users"):
                value = data.get(key)
                if isinstance(value, list):
                    ips.extend(str(item.get("ip", item)) if isinstance(item, dict) else str(item) for item in value)
        except Exception:
            pass
        ips.extend(re.findall(r"\b(?:\d{1,3}\.){3}\d{1,3}\b", text))
    if count <= 0:
        recent = query_recent_user_activity(email)
        if recent["online"]:
            count = recent["online_count"]
            ips = recent["online_ips"]
            source = "recent traffic"
    return {"online": count > 0, "online_count": count, "online_ips": sorted(set(ips)), "online_source": source}


def line_has_exact_email(line, email):
    return re.search(r"\bemail:\s+" + re.escape(email) + r"(?=\s|$)", line) is not None


def query_recent_user_activity(email, seconds=120):
    r = run(f"journalctl -u xray --since {shlex.quote(str(seconds) + ' seconds ago')} --no-pager", 5)
    if r["rc"] != 0:
        return {"online": False, "online_count": 0, "online_ips": []}
    ips = []
    for line in r["out"].splitlines():
        if " accepted " not in line or not line_has_exact_email(line, email):
            continue
        m = re.search(r" from (?:tcp:)?([0-9]{1,3}(?:\.[0-9]{1,3}){3}):", line)
        if m:
            ips.append(m.group(1))
    unique = sorted(set(ips))
    return {"online": bool(unique), "online_count": len(unique), "online_ips": unique}


def query_recent_destinations(email, seconds=300, limit=6):
    r = run(f"journalctl -u xray --since {shlex.quote(str(seconds) + ' seconds ago')} --output=short-iso --no-pager", 5)
    if r["rc"] != 0:
        return []
    seen = set()
    items = []
    for line in reversed(r["out"].splitlines()):
        if " accepted " not in line or not line_has_exact_email(line, email):
            continue
        m = re.search(r"^(\S+).*? from (?:tcp:)?([^ ]+) accepted (tcp|udp):([^ ]+) \[([^\]]+)\]", line)
        if not m:
            continue
        time_part, client, proto, dest, outbound = m.groups()
        if "T" in time_part:
            date_part, clock = time_part.split("T", 1)
            clock = clock.split("+", 1)[0].split(".", 1)[0]
            time_part = f"{date_part} {clock}"
        client_ip = client.rsplit(":", 1)[0]
        key = (dest, outbound)
        if key in seen:
            continue
        seen.add(key)
        items.append({
            "time": time_part,
            "client_ip": client_ip,
            "protocol": proto,
            "destination": dest,
            "outbound": outbound,
        })
        if len(items) >= limit:
            break
    return items


def update_persistent_usage():
    with STATE_LOCK:
        raw = query_user_usage()
        state = load_usage_state()
        totals = state.setdefault("totals", {})
        last_raw = state.setdefault("last_raw", {})
        for internal in INTERNAL_STATS_USERS:
            totals.pop(internal, None)
            last_raw.pop(internal, None)
        now = int(time.time())
        for email, values in raw.items():
            total_item = totals.setdefault(email, {"uplink": 0, "downlink": 0})
            last_item = last_raw.setdefault(email, {"uplink": 0, "downlink": 0})
            for direction in ("uplink", "downlink"):
                current = int(values.get(direction, 0) or 0)
                previous = int(last_item.get(direction, 0) or 0)
                delta = current - previous
                if delta < 0:
                    # Xray stats reset after restart. Preserve totals and start a new baseline.
                    delta = current
                if delta > 0:
                    total_item[direction] = int(total_item.get(direction, 0) or 0) + delta
                last_item[direction] = current
        state["updated_at"] = now
        save_usage_state(state)
        return state


def persistent_usage_totals():
    state = update_persistent_usage()
    result = {}
    for email, values in state.get("totals", {}).items():
        result[email] = {
            "uplink": int(values.get("uplink", 0) or 0),
            "downlink": int(values.get("downlink", 0) or 0),
        }
    return result


def usage_meta(raw=None):
    with STATE_LOCK:
        state = load_usage_state()
    updated = int(state.get("updated_at", 0) or 0)
    raw = raw if raw is not None else query_user_usage()
    return {
        "updated_at": updated,
        "updated_human": time.strftime("%Y-%m-%d %H:%M:%S", time.localtime(updated)) if updated else "",
        "tracked_users": len(state.get("totals", {}) or {}),
        "raw_users": len(raw or {}),
    }


def mask_id(value):
    if not value or len(value) < 13:
        return "hidden"
    return f"{value[:8]}...{value[-4:]}"


def xray_admin_config():
    cfg = load_xray_config()
    quotas = load_quotas()
    bans = load_bans()
    bandwidth = load_bandwidth_limits()
    usage = persistent_usage_totals() if stats_enabled(cfg) else {}
    meta = usage_meta() if stats_enabled(cfg) else {}
    users = {}
    for inbound in cfg.get("inbounds", []):
        if inbound.get("protocol") == "vless":
            for client in inbound.get("settings", {}).get("clients", []):
                email = client.get("email", "")
                if email:
                    user_id = client.get("id", "")
                    used = usage.get(email, {})
                    total = int(used.get("uplink", 0)) + int(used.get("downlink", 0))
                    limit_bytes = quotas.get(email, {}).get("limit_bytes")
                    bandwidth_item = bandwidth.get(email)
                    users[email] = {
                        "email": email,
                        "id_masked": mask_id(user_id),
                        "links": xray_connection_links(cfg, user_id, email) if user_id else [],
                        **query_online_user(email),
                        "usage_bytes": total,
                        "usage_human": human_bytes(total) if stats_enabled(cfg) else "stats disabled",
                        "limit_bytes": limit_bytes,
                        "limit_gb": round(limit_bytes / (1024 ** 3), 3) if limit_bytes else "",
                        "limit_human": human_bytes(limit_bytes) if limit_bytes else "none",
                        "quota_status": "over" if limit_bytes and total >= limit_bytes else ("ok" if limit_bytes else "unlimited"),
                        "bandwidth": bandwidth_item or {},
                        "bandwidth_human": bandwidth_label(bandwidth_item),
                    }
    direct_domains = []
    for rule in cfg.get("routing", {}).get("rules", []):
        if rule.get("outboundTag") == "direct" and isinstance(rule.get("domain"), list):
            direct_domains = sorted(rule["domain"])
            break
    banned_users = []
    now = int(time.time())
    for email, data in sorted(bans.items()):
        user_id = data.get("id", "")
        until = int(data.get("until", 0) or 0)
        banned_users.append({
            "email": email,
            "id_masked": mask_id(user_id),
            "until": until,
            "until_human": time.strftime("%Y-%m-%d %H:%M:%S", time.localtime(until)) if until else "unknown",
            "expired": bool(until and until <= now),
            "links": xray_connection_links(cfg, user_id, email) if user_id else [],
        })
    return {
        "users": sorted(users.values(), key=lambda x: x["email"]),
        "socks_users": managed_socks_users(cfg),
        "banned_users": banned_users,
        "direct_domains": direct_domains,
        "stats_enabled": stats_enabled(cfg),
        "usage_meta": meta,
    }


def read_json_body(handler):
    length = int(handler.headers.get("Content-Length", "0") or "0")
    if length <= 0 or length > 8192:
        return {}
    return json.loads(handler.rfile.read(length).decode("utf-8"))


def valid_email(value):
    return bool(re.fullmatch(r"[A-Za-z0-9_.@+-]{1,64}", value or ""))


def normalized_uuid(value):
    return str(uuid.UUID(value))


def public_server_ip():
    ip = run("ip -4 -o addr show dev eth0 | awk '{print $4}' | cut -d/ -f1 | head -n1", 3)["out"]
    return ip or "185.128.139.68"


def xray_connection_links(cfg, user_id, email):
    host = public_server_ip()
    links = []
    for inbound in cfg.get("inbounds", []):
        if inbound.get("protocol") != "vless":
            continue
        if not inbound_has_client(inbound, email, user_id):
            continue
        stream = inbound.get("streamSettings", {})
        network = stream.get("network", "tcp")
        listen = inbound.get("listen", "")
        port = inbound.get("port")
        external_port = 80 if listen == "127.0.0.1" and port == 3002 else port
        query = {"encryption": "none", "type": network, "security": stream.get("security") or "none"}
        if network == "ws":
            path = stream.get("wsSettings", {}).get("path", "/")
            query["path"] = path
            name = f"{email}-limited" if is_limited_inbound(inbound) else f"{email}-ws"
        elif network == "xhttp":
            path = stream.get("xhttpSettings", {}).get("path", "/xhttp")
            query["path"] = path
            name = f"{email}-xhttp"
        else:
            name = f"{email}-{network}"
        query_string = "&".join(f"{quote(str(k), safe='')}={quote(str(v), safe='')}" for k, v in query.items())
        url = f"vless://{user_id}@{host}:{external_port}?{query_string}#{quote(name, safe='')}"
        links.append({"name": name, "url": url})
    return links


def valid_socks_user(value):
    return bool(re.fullmatch(r"[A-Za-z0-9_.@+-]{1,64}", value or ""))


def safe_tag_value(value):
    return re.sub(r"[^A-Za-z0-9_.@+-]+", "-", value or "user").strip("-")[:48] or "user"


def generated_password():
    return uuid.uuid4().hex[:16]


def socks_account(inbound):
    accounts = inbound.get("settings", {}).get("accounts", [])
    if not accounts:
        return None
    account = accounts[0]
    user = account.get("user", "")
    password = account.get("pass", "")
    if not user or not password:
        return None
    return {"user": user, "pass": password}


def is_public_managed_socks(inbound):
    if inbound.get("protocol") != "socks":
        return False
    if inbound.get("listen") in ("127.0.0.1", "::1", "localhost"):
        return False
    if inbound.get("settings", {}).get("auth") != "password":
        return False
    tag = str(inbound.get("tag") or "")
    return tag.startswith("managed-socks-") or int(inbound.get("port") or 0) == 1080


def managed_socks_inbounds(cfg):
    return [inbound for inbound in cfg.get("inbounds", []) if is_public_managed_socks(inbound) and socks_account(inbound)]


def find_socks_inbound(cfg, user):
    for inbound in managed_socks_inbounds(cfg):
        account = socks_account(inbound)
        if account and account.get("user") == user:
            return inbound
    return None


def socks_connection_links(user, password, port):
    host = public_server_ip()
    auth = f"{quote(user, safe='')}:{quote(password, safe='')}"
    name = quote(f"{user}-socks", safe="")
    return [
        {"name": "SOCKS5 URI", "url": f"socks5://{auth}@{host}:{int(port)}#{name}"},
        {"name": "Host", "url": f"{host}:{int(port)}"},
        {"name": "Username", "url": user},
        {"name": "Password", "url": password},
    ]


def socks_inbound_for(user, password, port):
    return {
        "tag": f"managed-socks-{safe_tag_value(user)}",
        "listen": "0.0.0.0",
        "port": int(port),
        "protocol": "socks",
        "settings": {
            "auth": "password",
            "accounts": [{"user": user, "pass": password}],
            "udp": True,
        },
        "sniffing": {"enabled": True, "destOverride": ["http", "tls", "quic"]},
    }


def next_socks_port(cfg):
    ports = used_ports(cfg)
    for port in range(SOCKS_PORT_START, SOCKS_PORT_END + 1):
        if port not in ports:
            return port
    raise ValueError("no free SOCKS port available")


def parse_socks_port(value, cfg, old_port=None):
    if value in (None, ""):
        return int(old_port or next_socks_port(cfg))
    port = int(value)
    if port < 1 or port > 65535:
        raise ValueError("invalid port")
    ports = used_ports(cfg)
    if old_port:
        ports.discard(int(old_port))
    if port in ports:
        raise ValueError("port is already in use")
    return port


def load_socks_usage_state():
    try:
        with open(SOCKS_USAGE_FILE) as f:
            data = json.load(f)
        if isinstance(data, dict):
            data.setdefault("totals", {})
            data.setdefault("last_raw", {})
            return data
    except Exception:
        pass
    return {"totals": {}, "last_raw": {}}


def save_socks_usage_state(data):
    tmp = SOCKS_USAGE_FILE + ".tmp"
    with open(tmp, "w") as f:
        json.dump(data, f, indent=2, sort_keys=True)
        f.write("\n")
    os.replace(tmp, SOCKS_USAGE_FILE)


def nft_socks_counters():
    r = run("nft -j list table inet xray_socks_usage", 4)
    if r["rc"] != 0:
        return {}
    try:
        data = json.loads(r["out"] or "{}")
    except Exception:
        return {}
    counters = {}
    for item in data.get("nftables", []):
        rule = item.get("rule")
        if not rule:
            continue
        comment = str(rule.get("comment") or "")
        bytes_value = None
        for expr in rule.get("expr", []):
            if "comment" in expr:
                comment = str(expr.get("comment") or "")
            if "counter" in expr:
                bytes_value = int(expr["counter"].get("bytes", 0) or 0)
        if comment.startswith("socks:") and bytes_value is not None:
            counters[comment] = bytes_value
    return counters


def update_socks_usage_totals():
    with STATE_LOCK:
        raw = nft_socks_counters()
        state = load_socks_usage_state()
        totals = state.setdefault("totals", {})
        last_raw = state.setdefault("last_raw", {})
        now = int(time.time())
        for key, current in raw.items():
            parts = key.split(":", 2)
            if len(parts) != 3:
                continue
            _, user, direction = parts
            bucket = "download" if direction.startswith("out") else "upload"
            item = totals.setdefault(user, {"upload": 0, "download": 0})
            previous = int(last_raw.get(key, 0) or 0)
            delta = int(current or 0) - previous
            if delta < 0:
                delta = int(current or 0)
            if delta > 0:
                item[bucket] = int(item.get(bucket, 0) or 0) + delta
            last_raw[key] = int(current or 0)
        state["updated_at"] = now
        save_socks_usage_state(state)
        return state


def sync_socks_usage_rules(cfg=None):
    try:
        update_socks_usage_totals()
    except Exception:
        pass
    cfg = cfg or load_xray_config()
    rules = []
    for inbound in managed_socks_inbounds(cfg):
        account = socks_account(inbound)
        if not account:
            continue
        user = account["user"]
        port = int(inbound.get("port"))
        comment_user = user.replace('"', "").replace(":", "-")
        rules.extend([
            f"add rule inet xray_socks_usage prerouting tcp dport {port} counter comment \"socks:{comment_user}:in\"",
            f"add rule inet xray_socks_usage prerouting udp dport {port} counter comment \"socks:{comment_user}:in-udp\"",
            f"add rule inet xray_socks_usage output tcp sport {port} counter comment \"socks:{comment_user}:out\"",
            f"add rule inet xray_socks_usage output udp sport {port} counter comment \"socks:{comment_user}:out-udp\"",
        ])
    script = [
        "destroy table inet xray_socks_usage",
        "add table inet xray_socks_usage",
        "add chain inet xray_socks_usage prerouting { type filter hook prerouting priority -150; policy accept; }",
        "add chain inet xray_socks_usage output { type filter hook output priority -150; policy accept; }",
        *rules,
    ]
    r = run("printf '%s\n' " + " ".join(shlex.quote(line) for line in script) + " | nft -f -", 5)
    if r["rc"] != 0:
        raise ValueError("failed to sync SOCKS usage counters: " + (r["err"] or r["out"]))


def allow_socks_firewall_port(port):
    port = int(port)
    for proto in ("tcp", "udp"):
        run(f"ufw allow {port}/{proto} comment 'managed SOCKS'", 8)


def delete_socks_firewall_port(port):
    port = int(port)
    for proto in ("tcp", "udp"):
        run(f"ufw --force delete allow {port}/{proto}", 8)


def socks_port_still_used(cfg, port):
    port = int(port)
    for inbound in managed_socks_inbounds(cfg):
        if int(inbound.get("port") or 0) == port:
            return True
    return False


def socks_usage_totals():
    state = update_socks_usage_totals()
    return state.get("totals", {}) if isinstance(state.get("totals"), dict) else {}


def normalize_peer_ip(value):
    value = (value or "").strip()
    value = value.removeprefix("[::ffff:").removesuffix("]")
    value = value.strip("[]")
    if value.startswith("::ffff:"):
        value = value.split("::ffff:", 1)[1]
    return value


def query_socks_online(port):
    port = int(port)
    r = run(f"ss -Hnt state established 'sport = :{port}' 2>/dev/null", 4)
    ips = []
    sessions = 0
    if r["rc"] == 0 and r["out"]:
        for line in r["out"].splitlines():
            parts = line.split()
            if len(parts) < 4:
                continue
            peer = parts[-1]
            if peer.startswith("["):
                host = peer.rsplit("]:", 1)[0] + "]"
            else:
                host = peer.rsplit(":", 1)[0]
            ip = normalize_peer_ip(host)
            if ip:
                ips.append(ip)
                sessions += 1
    unique_ips = sorted(set(ips))
    return {
        "online": sessions > 0,
        "online_count": sessions,
        "online_ips": unique_ips,
        "online_source": "active socket",
    }


def managed_socks_users(cfg):
    usage = socks_usage_totals()
    host = public_server_ip()
    result = []
    for inbound in managed_socks_inbounds(cfg):
        account = socks_account(inbound)
        user = account["user"]
        port = int(inbound.get("port"))
        used = usage.get(user, {})
        upload = int(used.get("upload", 0) or 0)
        download = int(used.get("download", 0) or 0)
        online = query_socks_online(port)
        result.append({
            "user": user,
            "tag": inbound.get("tag", ""),
            "host": host,
            "port": port,
            **online,
            "upload_bytes": upload,
            "download_bytes": download,
            "usage_bytes": upload + download,
            "upload_human": human_bytes(upload),
            "download_human": human_bytes(download),
            "usage_human": human_bytes(upload + download),
            "links": socks_connection_links(user, account["pass"], port),
        })
    return sorted(result, key=lambda x: (x["port"], x["user"]))


def normalize_domain(value):
    value = (value or "").strip().lower()
    value = value.removeprefix("http://").removeprefix("https://").split("/", 1)[0]
    value = value.removeprefix("domain:")
    if not re.fullmatch(r"(\*\.)?[a-z0-9.-]+\.[a-z0-9-]{2,}", value):
        raise ValueError("invalid domain")
    return "domain:" + value.removeprefix("*.")


def direct_domain_rule(cfg):
    for rule in cfg.get("routing", {}).get("rules", []):
        if rule.get("outboundTag") == "direct" and isinstance(rule.get("domain"), list):
            return rule
    raise ValueError("direct domain rule not found")


def used_ports(cfg):
    ports = set()
    for inbound in cfg.get("inbounds", []):
        try:
            ports.add(int(inbound.get("port")))
        except Exception:
            pass
    for item in load_bandwidth_limits().values():
        try:
            ports.add(int(item.get("port")))
        except Exception:
            pass
    text = run("ss -ltnH | awk '{print $4}'", 3)["out"]
    for token in text.splitlines():
        if ":" in token:
            try:
                ports.add(int(token.rsplit(":", 1)[1]))
            except Exception:
                pass
    return ports


def next_limited_port(cfg):
    ports = used_ports(cfg)
    for port in range(21000, 22000):
        if port not in ports:
            return port
    raise ValueError("no free limited inbound port available")


def bandwidth_label(item):
    if not item:
        return "none"
    down = float(item.get("download_mbps", 0) or 0)
    up = float(item.get("upload_mbps", 0) or 0)
    port = item.get("port", "")
    return f"down {down:g} Mbps / up {up:g} Mbps on port {port}"


def limited_inbound_for(email, user_id, port):
    return {
        "tag": f"limited-{email}",
        "listen": "0.0.0.0",
        "port": int(port),
        "protocol": "vless",
        "settings": {"clients": [{"id": user_id, "email": email}], "decryption": "none"},
        "streamSettings": {"network": "ws", "security": "none", "wsSettings": {"path": f"/limited/{email}"}},
        "sniffing": {"enabled": True, "destOverride": ["http", "tls", "quic"]},
    }


def remove_limited_inbound(cfg, email):
    before = len(cfg.get("inbounds", []))
    cfg["inbounds"] = [
        inbound for inbound in cfg.get("inbounds", [])
        if not (is_limited_inbound(inbound) and (inbound.get("tag") == f"limited-{email}" or inbound_has_client(inbound, email)))
    ]
    return before - len(cfg.get("inbounds", []))


def upsert_limited_inbound(cfg, email, user_id, port):
    remove_limited_inbound(cfg, email)
    cfg.setdefault("inbounds", []).append(limited_inbound_for(email, user_id, port))


def apply_bandwidth_service(enable=True):
    if enable:
        run_async("systemctl enable --now xray-bandwidth-limits.service >/dev/null 2>&1; systemctl restart xray-bandwidth-limits.service")
    else:
        run_async("systemctl restart xray-bandwidth-limits.service")


def backup_and_apply(cfg, restart_delay=1):
    stamp = time.strftime("%Y%m%d-%H%M%S-xray-admin")
    backup_dir = f"/root/xray-audit-backups/{stamp}"
    os.makedirs(backup_dir, exist_ok=True)
    run(f"cp -a /usr/local/etc/xray/config.json {backup_dir}/config.json", 5)
    tmp = f"/tmp/xray-admin-{stamp}.json"
    with open(tmp, "w") as f:
        json.dump(cfg, f, ensure_ascii=False, indent=2)
        f.write("\n")
    test = run(f"xray run -test -config {tmp}", 10)
    if test["rc"] != 0:
        raise ValueError("xray config test failed: " + (test["err"] or test["out"]))
    run(f"install -m 0644 {tmp} /usr/local/etc/xray/config.json", 5)
    run_async(f"sleep {int(restart_delay)}; systemctl restart xray.service")
    return backup_dir


def xray_admin_action(action, body):
    cfg = load_xray_config()
    if action == "add-user":
        email = (body.get("email") or "").strip()
        if not valid_email(email):
            raise ValueError("invalid email")
        if email in load_bans():
            raise ValueError("user is temporarily banned; unban first")
        user_id = normalized_uuid(body.get("id") or str(uuid.uuid4()))
        touched = 0
        for inbound in cfg.get("inbounds", []):
            if inbound.get("protocol") != "vless":
                continue
            if is_limited_inbound(inbound):
                continue
            clients = inbound.get("settings", {}).setdefault("clients", [])
            if any(c.get("email") == email for c in clients):
                raise ValueError("user already exists")
            clients.append({"id": user_id, "email": email})
            touched += 1
        if touched == 0:
            raise ValueError("no vless inbound found")
        links = xray_connection_links(cfg, user_id, email)
        backup = backup_and_apply(cfg)
        return {"ok": True, "email": email, "id_masked": mask_id(user_id), "links": links, "backup": backup}
    if action == "edit-user":
        old_email = (body.get("old_email") or "").strip()
        email = (body.get("email") or old_email).strip()
        if not valid_email(old_email) or not valid_email(email):
            raise ValueError("invalid email")
        if email != old_email and find_vless_user(cfg, email):
            raise ValueError("target email already exists")
        user_id = normalized_uuid(body["id"]) if body.get("id") else None
        touched = 0
        for inbound in cfg.get("inbounds", []):
            if inbound.get("protocol") != "vless":
                continue
            for client in inbound.get("settings", {}).get("clients", []):
                if client.get("email") == old_email:
                    client["email"] = email
                    if user_id:
                        client["id"] = user_id
                    touched += 1
        if touched == 0:
            raise ValueError("user not found")
        bandwidth = load_bandwidth_limits()
        if old_email in bandwidth:
            bandwidth[email] = bandwidth.pop(old_email)
            bandwidth[email]["email"] = email
            save_bandwidth_limits(bandwidth)
            apply_bandwidth_service(enable=True)
        backup = backup_and_apply(cfg)
        rename_user_state(old_email, email)
        return {"ok": True, "email": email, "backup": backup}
    if action == "delete-user":
        email = (body.get("email") or "").strip()
        if not valid_email(email):
            raise ValueError("invalid email")
        touched = remove_vless_user(cfg, email)
        if touched == 0:
            raise ValueError("user not found")
        bans = load_bans()
        bans.pop(email, None)
        save_bans(bans)
        bandwidth = load_bandwidth_limits()
        if email in bandwidth:
            bandwidth.pop(email, None)
            save_bandwidth_limits(bandwidth)
            apply_bandwidth_service(enable=bool(bandwidth))
        remove_limited_inbound(cfg, email)
        backup = backup_and_apply(cfg)
        delete_user_state(email)
        return {"ok": True, "email": email, "backup": backup}
    if action == "temp-ban-user":
        email = (body.get("email") or "").strip()
        if not valid_email(email):
            raise ValueError("invalid email")
        minutes = float(body.get("minutes") or 0)
        if minutes <= 0:
            raise ValueError("duration must be positive")
        user = find_vless_user(cfg, email)
        if not user or not user.get("id"):
            raise ValueError("user not found")
        touched = remove_vless_user(cfg, email)
        if touched == 0:
            raise ValueError("user not found")
        remove_limited_inbound(cfg, email)
        until = int(time.time() + minutes * 60)
        bans = load_bans()
        bans[email] = {"id": user["id"], "email": email, "until": until}
        save_bans(bans)
        backup = backup_and_apply(cfg)
        return {
            "ok": True,
            "email": email,
            "until": until,
            "until_human": time.strftime("%Y-%m-%d %H:%M:%S", time.localtime(until)),
            "backup": backup,
        }
    if action == "unban-user":
        email = (body.get("email") or "").strip()
        if not valid_email(email):
            raise ValueError("invalid email")
        bans = load_bans()
        data = bans.get(email)
        if not data:
            raise ValueError("user is not banned")
        user_id = data.get("id", "")
        if not user_id:
            raise ValueError("banned user has no stored id")
        add_vless_user(cfg, email, user_id)
        bandwidth = load_bandwidth_limits()
        if email in bandwidth:
            upsert_limited_inbound(cfg, email, user_id, bandwidth[email]["port"])
        bans.pop(email, None)
        save_bans(bans)
        backup = backup_and_apply(cfg)
        return {"ok": True, "email": email, "backup": backup}
    if action == "add-socks-user":
        user = (body.get("user") or "").strip()
        if not valid_socks_user(user):
            raise ValueError("invalid SOCKS username")
        if find_socks_inbound(cfg, user):
            raise ValueError("SOCKS user already exists")
        password = (body.get("password") or "").strip() or generated_password()
        if len(password) < 6 or len(password) > 128:
            raise ValueError("password must be 6-128 characters")
        port = parse_socks_port(body.get("port"), cfg)
        cfg.setdefault("inbounds", []).append(socks_inbound_for(user, password, port))
        backup = backup_and_apply(cfg)
        allow_socks_firewall_port(port)
        sync_socks_usage_rules(cfg)
        return {"ok": True, "user": user, "port": port, "links": socks_connection_links(user, password, port), "backup": backup}
    if action == "edit-socks-user":
        old_user = (body.get("old_user") or "").strip()
        user = (body.get("user") or old_user).strip()
        if not valid_socks_user(old_user) or not valid_socks_user(user):
            raise ValueError("invalid SOCKS username")
        inbound = find_socks_inbound(cfg, old_user)
        if not inbound:
            raise ValueError("SOCKS user not found")
        duplicate = find_socks_inbound(cfg, user)
        if duplicate is not None and duplicate is not inbound:
            raise ValueError("target SOCKS user already exists")
        old_port = int(inbound.get("port"))
        port = parse_socks_port(body.get("port"), cfg, old_port=old_port)
        account = socks_account(inbound)
        password = (body.get("password") or "").strip() or account["pass"]
        if len(password) < 6 or len(password) > 128:
            raise ValueError("password must be 6-128 characters")
        inbound["tag"] = f"managed-socks-{safe_tag_value(user)}"
        inbound["listen"] = inbound.get("listen") or "0.0.0.0"
        inbound["port"] = port
        inbound.setdefault("settings", {})["auth"] = "password"
        inbound["settings"]["accounts"] = [{"user": user, "pass": password}]
        inbound["settings"]["udp"] = True
        inbound.setdefault("sniffing", {"enabled": True, "destOverride": ["http", "tls", "quic"]})
        if old_user != user:
            state = load_socks_usage_state()
            for bucket in ("totals",):
                values = state.setdefault(bucket, {})
                if old_user in values:
                    old = values.pop(old_user)
                    existing = values.get(user, {})
                    values[user] = {
                        "upload": int(existing.get("upload", 0) or 0) + int(old.get("upload", 0) or 0),
                        "download": int(existing.get("download", 0) or 0) + int(old.get("download", 0) or 0),
                    }
            save_socks_usage_state(state)
        backup = backup_and_apply(cfg)
        allow_socks_firewall_port(port)
        if old_port != port and not socks_port_still_used(cfg, old_port):
            delete_socks_firewall_port(old_port)
        sync_socks_usage_rules(cfg)
        return {"ok": True, "user": user, "port": port, "links": socks_connection_links(user, password, port), "backup": backup}
    if action == "delete-socks-user":
        user = (body.get("user") or "").strip()
        if not valid_socks_user(user):
            raise ValueError("invalid SOCKS username")
        inbound = find_socks_inbound(cfg, user)
        deleted_port = int(inbound.get("port")) if inbound else 0
        before = len(cfg.get("inbounds", []))
        cfg["inbounds"] = [
            inbound for inbound in cfg.get("inbounds", [])
            if not (is_public_managed_socks(inbound) and (socks_account(inbound) or {}).get("user") == user)
        ]
        if len(cfg.get("inbounds", [])) == before:
            raise ValueError("SOCKS user not found")
        backup = backup_and_apply(cfg)
        if deleted_port and not socks_port_still_used(cfg, deleted_port):
            delete_socks_firewall_port(deleted_port)
        sync_socks_usage_rules(cfg)
        return {"ok": True, "user": user, "backup": backup}
    if action == "add-direct-domain":
        domain = normalize_domain(body.get("domain"))
        rule = direct_domain_rule(cfg)
        if domain not in rule["domain"]:
            rule["domain"].append(domain)
            rule["domain"] = sorted(rule["domain"])
        backup = backup_and_apply(cfg)
        return {"ok": True, "domain": domain, "backup": backup}
    if action == "set-user-limit":
        email = (body.get("email") or "").strip()
        if not valid_email(email):
            raise ValueError("invalid email")
        raw = str(body.get("limit_gb") or "").strip()
        quotas = load_quotas()
        if raw == "":
            quotas.pop(email, None)
            save_quotas(quotas)
            return {"ok": True, "email": email, "limit_bytes": None}
        limit_gb = float(raw)
        if limit_gb <= 0:
            raise ValueError("limit must be positive")
        quotas[email] = {"limit_bytes": int(limit_gb * 1024 * 1024 * 1024)}
        save_quotas(quotas)
        cfg = load_xray_config()
        if ensure_stats_config(cfg):
            backup = backup_and_apply(cfg)
        else:
            backup = None
        return {"ok": True, "email": email, "limit_bytes": quotas[email]["limit_bytes"], "backup": backup}
    if action == "set-bandwidth-limit":
        email = (body.get("email") or "").strip()
        if not valid_email(email):
            raise ValueError("invalid email")
        user = find_vless_user(cfg, email)
        if not user or not user.get("id"):
            raise ValueError("user not found")
        raw_down = str(body.get("download_mbps") or "").strip()
        raw_up = str(body.get("upload_mbps") or "").strip()
        bandwidth = load_bandwidth_limits()
        if raw_down == "" and raw_up == "":
            new_bandwidth = dict(bandwidth)
            new_bandwidth.pop(email, None)
            remove_limited_inbound(cfg, email)
            backup = backup_and_apply(cfg)
            save_bandwidth_limits(new_bandwidth)
            apply_bandwidth_service(enable=bool(new_bandwidth))
            return {"ok": True, "email": email, "bandwidth": None, "backup": backup}
        down = float(raw_down or 0)
        up = float(raw_up or 0)
        if down <= 0 and up <= 0:
            raise ValueError("at least one speed must be positive")
        if down > 1000 or up > 1000:
            raise ValueError("speed limit is too high")
        port = int((bandwidth.get(email) or {}).get("port") or next_limited_port(cfg))
        new_bandwidth = dict(bandwidth)
        new_bandwidth[email] = {"email": email, "port": port, "download_mbps": down, "upload_mbps": up}
        upsert_limited_inbound(cfg, email, user["id"], port)
        links = xray_connection_links(cfg, user["id"], email)
        backup = backup_and_apply(cfg)
        save_bandwidth_limits(new_bandwidth)
        apply_bandwidth_service(enable=True)
        return {"ok": True, "email": email, "bandwidth": new_bandwidth[email], "links": links, "backup": backup}
    if action == "delete-direct-domain":
        domain = body.get("domain") or ""
        if not domain.startswith(("domain:", "full:", "regexp:", "geosite:")):
            domain = normalize_domain(domain)
        rule = direct_domain_rule(cfg)
        if domain not in rule["domain"]:
            raise ValueError("domain not found")
        rule["domain"] = [d for d in rule["domain"] if d != domain]
        backup = backup_and_apply(cfg)
        return {"ok": True, "domain": domain, "backup": backup}
    raise ValueError("unknown action")


def enforce_quotas_once():
    try:
        cfg = load_xray_config()
        quotas = load_quotas()
        if not quotas:
            return
        if ensure_stats_config(cfg):
            backup_and_apply(cfg, restart_delay=1)
            return
        usage = persistent_usage_totals()
        remove = []
        for email, quota in quotas.items():
            limit = quota.get("limit_bytes")
            used = usage.get(email, {}).get("uplink", 0) + usage.get(email, {}).get("downlink", 0)
            if limit and used >= limit:
                remove.append(email)
        if not remove:
            return
        changed = False
        for inbound in cfg.get("inbounds", []):
            if inbound.get("protocol") != "vless":
                continue
            clients = inbound.get("settings", {}).get("clients", [])
            kept = [c for c in clients if c.get("email") not in remove]
            if len(kept) != len(clients):
                inbound["settings"]["clients"] = kept
                changed = True
        if changed:
            backup_and_apply(cfg, restart_delay=1)
    except Exception:
        pass


def enforce_bans_once():
    try:
        bans = load_bans()
        if not bans:
            return
        now = int(time.time())
        expired = [email for email, data in bans.items() if int(data.get("until", 0) or 0) <= now]
        if not expired:
            return
        cfg = load_xray_config()
        bandwidth = load_bandwidth_limits()
        changed = False
        for email in expired:
            data = bans.get(email, {})
            user_id = data.get("id")
            if user_id:
                changed = bool(add_vless_user(cfg, email, user_id)) or changed
                if email in bandwidth:
                    upsert_limited_inbound(cfg, email, user_id, bandwidth[email]["port"])
                    changed = True
            bans.pop(email, None)
        save_bans(bans)
        if changed:
            backup_and_apply(cfg, restart_delay=1)
    except Exception:
        pass


def quota_loop():
    while True:
        enforce_bans_once()
        try:
            cfg = load_xray_config()
            if stats_enabled(cfg):
                update_persistent_usage()
            update_socks_usage_totals()
        except Exception:
            pass
        enforce_quotas_once()
        time.sleep(60)


def status():
    services = service_statuses([
        "xray",
        "nginx",
        "sub",
        "openvpn@company",
        "openvpn@client-tun1",
        "xray-ai-route-failover",
        "xray-bandwidth-limits",
        "vpn-monitor",
        "tun0-watchdog",
        "xray-stack-monitor",
        "xray2",
        "xray-icmp",
    ], disabled_names=("xray2", "xray-icmp", "xray-bandwidth-limits", "tun0-watchdog"))
    return {
        "generated_at": time.strftime("%Y-%m-%d %H:%M:%S %z"),
        "hostname": run("hostname", 2)["out"],
        "system": system_metrics(),
        "traffic": traffic_totals(),
        "proxy_interface": proxy_interface(),
        "tunnels": {"tun0": tunnel_counters("tun0"), "tun1": tunnel_counters("tun1")},
        "services": services,
        "interfaces": {"tun0": iface("tun0"), "tun1": iface("tun1"), "eth0": iface("eth0")},
        "ai_route": ai_route_status(),
        "routes": {
            "cloudflare": run(
                "ip -details route show 104.16.0.0/13; "
                "ip -details route show 172.64.0.0/13; "
                "ip route get 172.64.155.209 2>/dev/null",
                4,
            )["out"]
        },
        "checks": {
            "xray_config": {
                "ok": cached_xray_config_ok()
            },
            "proxy_upstream": proxy_upstream_health(),
            "xray_socks": xray_socks_health(),
        },
        "logs": {
            "monitor": run(
                "tail -n 12 /var/log/vpn-monitor.log 2>/dev/null; "
                "tail -n 8 /var/log/tun0-watchdog.log 2>/dev/null",
                3,
            )["out"]
        },
    }


def run_named_test(name):
    user, password = socks_creds()
    proxy = (
        f"--socks5-hostname {user}:{password}@127.0.0.1:1080"
        if user and password
        else "--socks5-hostname 127.0.0.1:1080"
    )
    tests = {
        "socks_google": (
            f"curl -k --connect-timeout 5 --max-time 10 -sS -o /dev/null "
            f"-w 'http=%{{http_code}}|ip=%{{remote_ip}}|time=%{{time_total}}|err=%{{errormsg}}' {proxy} https://google.com/"
        ),
        "telegram_xray": (
            f"curl -k --connect-timeout 5 --max-time 10 -sS -o /dev/null "
            f"-w 'http=%{{http_code}}|ip=%{{remote_ip}}|time=%{{time_total}}|err=%{{errormsg}}' {proxy} https://telegram.org/"
        ),
        "direct_blubank": (
            "curl --interface eth0 -I --connect-timeout 5 --max-time 10 -sS -o /dev/null "
            "-w 'http=%{http_code}|ip=%{remote_ip}|time=%{time_total}|err=%{errormsg}' https://blubank.com/"
        ),
        "tun1_chatgpt": (
            "curl --interface tun1 -I --connect-timeout 5 --max-time 10 -sS -o /dev/null "
            "-w 'http=%{http_code}|ip=%{remote_ip}|time=%{time_total}|err=%{errormsg}' https://chatgpt.com/"
        ),
    }
    if name not in tests:
        return {"status": "error", "label": "Invalid test", "detail": name}
    return human_http_result(curl_summary(tests[name]))


def run_tunnel_target_test(target, port, interface):
    target = (target or "").strip().lower()
    interface = (interface or "").strip()
    try:
        port_int = int(port or 443)
    except Exception:
        port_int = 443
    if interface not in ("tun0", "tun1", "eth0"):
        return {"status": "error", "label": "Invalid interface", "detail": interface}
    if port_int < 1 or port_int > 65535:
        return {"status": "error", "label": "Invalid port", "detail": str(port)}
    if not re.fullmatch(r"[a-z0-9.-]+|\d{1,3}(?:\.\d{1,3}){3}", target or ""):
        return {"status": "error", "label": "Invalid target", "detail": target}
    url_host = target
    if re.fullmatch(r"\d{1,3}(?:\.\d{1,3}){3}", target):
        url_host = target
    scheme = "https" if port_int == 443 else "http"
    cmd = (
        f"curl --interface {shlex.quote(interface)} -k -I --connect-timeout 6 --max-time 10 -sS -o /dev/null "
        f"-w 'http=%{{http_code}}|ip=%{{remote_ip}}|time=%{{time_total}}|err=%{{errormsg}}' "
        f"{shlex.quote(f'{scheme}://{url_host}:{port_int}/')}"
    )
    result = human_http_result(curl_summary(cmd))
    if result.get("status") == "blocked" and result.get("label", "").startswith("Unexpected response"):
        result["status"] = "ok"
        result["label"] = result["label"].replace("Unexpected response", "Reachable - HTTP")
    result["detail"] = f"{interface} -> {target}:{port_int} | {result.get('detail', '')}"
    return result


class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        return

    def send_body(self, code, body, content_type):
        raw = body.encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", content_type)
        self.send_header("Cache-Control", "no-store")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        try:
            self.wfile.write(raw)
        except (BrokenPipeError, ConnectionResetError):
            pass

    def do_GET(self):
        path = self.path.split("?", 1)[0]
        if path in ("/", "/monitor", "/monitor/", "/index.html"):
            self.send_body(200, HTML, "text/html; charset=utf-8")
        elif path in ("/api/status", "/monitor/api/status"):
            self.send_body(200, json.dumps(status(), ensure_ascii=False), "application/json; charset=utf-8")
        elif path in ("/api/test", "/monitor/api/test"):
            name = ""
            if "?" in self.path:
                for part in self.path.split("?", 1)[1].split("&"):
                    if part.startswith("name="):
                        name = part.split("=", 1)[1]
            self.send_body(200, json.dumps(run_named_test(name), ensure_ascii=False), "application/json; charset=utf-8")
        elif path in ("/api/tunnel-test", "/monitor/api/tunnel-test"):
            qs = parse_qs(urlparse(self.path).query)
            body = run_tunnel_target_test(
                qs.get("target", [""])[0],
                qs.get("port", ["443"])[0],
                qs.get("interface", ["tun0"])[0],
            )
            self.send_body(200, json.dumps(body, ensure_ascii=False), "application/json; charset=utf-8")
        elif path in ("/api/xray/config", "/monitor/api/xray/config"):
            self.send_body(200, json.dumps(xray_admin_config(), ensure_ascii=False), "application/json; charset=utf-8")
        elif path in ("/api/xray/raw-config", "/monitor/api/xray/raw-config"):
            with open("/usr/local/etc/xray/config.json") as f:
                raw = json.dumps(json.load(f), indent=2, ensure_ascii=False)
            self.send_body(200, raw + "\n", "application/json; charset=utf-8")
        elif path in ("/api/xray/activity", "/monitor/api/xray/activity"):
            qs = parse_qs(urlparse(self.path).query)
            email = qs.get("email", [""])[0]
            seconds = int(qs.get("seconds", ["60"])[0] or "60")
            if not valid_email(email):
                self.send_body(400, json.dumps({"ok": False, "error": "invalid email"}), "application/json; charset=utf-8")
            else:
                seconds = max(10, min(seconds, 600))
                body = {
                    "ok": True,
                    "email": email,
                    "seconds": seconds,
                    "generated_at": time.strftime("%Y-%m-%d %H:%M:%S %z"),
                    "items": query_recent_destinations(email, seconds=seconds, limit=80),
                }
                self.send_body(200, json.dumps(body, ensure_ascii=False), "application/json; charset=utf-8")
        else:
            self.send_body(404, "not found", "text/plain; charset=utf-8")

    def do_POST(self):
        path = urlparse(self.path).path
        prefix = "/monitor/api/xray/"
        alt_prefix = "/api/xray/"
        if path.startswith(prefix):
            action = path[len(prefix):]
        elif path.startswith(alt_prefix):
            action = path[len(alt_prefix):]
        else:
            self.send_body(404, "not found", "text/plain; charset=utf-8")
            return
        try:
            result = xray_admin_action(action, read_json_body(self))
            self.send_body(200, json.dumps(result, ensure_ascii=False), "application/json; charset=utf-8")
        except Exception as exc:
            self.send_body(400, json.dumps({"ok": False, "error": str(exc)}, ensure_ascii=False), "application/json; charset=utf-8")


if __name__ == "__main__":
    try:
        sync_socks_usage_rules(load_xray_config())
    except Exception:
        pass
    threading.Thread(target=quota_loop, daemon=True).start()
    ThreadingHTTPServer((HOST, PORT), Handler).serve_forever()
