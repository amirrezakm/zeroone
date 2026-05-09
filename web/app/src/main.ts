import './styles.css';

type Link = { name: string; url: string };
type UserItem = { email:string; uuid:string; enabled:boolean; banned_until:number; quota_bytes:number; download_mbps:number; upload_mbps:number; bandwidth_port:number; links:Link[] };
type SocksItem = { name:string; listen:string; port:number; username:string; password:string; links:Link[] };
type Summary = {
  public_ip: string;
  users: number;
  socks: number;
  allow_apply: boolean;
  user_items: UserItem[];
  socks_items: SocksItem[];
  direct_domains: string[];
  block_domains: string[];
  manual_blocks: string[];
  tunnels: Array<{name:string; type:string; interface:string; priority:number}>;
  failover: {enabled:boolean; probe_ip:string; probe_port:number; cooldown_seconds:number};
};
type Health = { ok: boolean; generated_at: string; tunnels: Array<{name:string; interface:string; up:boolean; healthy:boolean; ipv4?:string; probe?:string; latency_ms?:number; error?:string}> };
type ApplyPlan = { ok: boolean; valid: boolean; changed: boolean; config_path: string; allow_apply: boolean; error?: string };
type Usage = { updated_at: number; users: Array<{email:string; uplink:number; downlink:number; total:number}> };
type QuotaPlan = { generated_at: number; actions: Array<{email:string; used_bytes:number; quota_bytes:number; action:string; reason:string}> };
type BandwidthPlan = { device: string; limits: Array<{email:string; port:number; download_mbps:number; upload_mbps:number}>; needs_apply: boolean; apply_locked: boolean; tc_commands: string[] };
type SystemInfo = { cpu:{percent:number; detail:string}; ram:{percent:number; detail:string; used_bytes:number; total_bytes:number}; tunnels:Array<{name:string; rx_bytes:number; tx_bytes:number}>; updated_at:number };
type FailoverMode = { outbound_tag:string; interface?:string };
type FailoverDecision = { decision:{current:FailoverMode; desired:FailoverMode; effective:FailoverMode; pending:boolean; confirmation_count:number; reason:string} };

const apiBase = import.meta.env.VITE_API_BASE || '';
const app = document.querySelector<HTMLDivElement>('#app')!;
let latestSummary: Summary | undefined;

async function fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiBase}${path}`, {cache: 'no-store', ...init});
  const body = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body.error || `${response.status} ${response.statusText}`);
  return body;
}
function post(path: string, body: unknown) { return fetchJSON(path, {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)}); }
function put(path: string, body: unknown) { return fetchJSON(path, {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)}); }
function badge(ok: boolean, label: string) { return `<span class="badge ${ok ? 'ok' : 'bad'}"><span></span>${label}</span>`; }
function esc(value: unknown) { return String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]!)); }
function bytes(value: number) {
  const units = ['B','KB','MB','GB','TB']; let n = value || 0; let i = 0;
  while (n >= 1024 && i < units.length - 1) { n /= 1024; i++; }
  return `${n.toFixed(i ? 1 : 0)} ${units[i]}`;
}
function linkText(links: Link[]) { return (links || []).map(l => `${l.name}\n${l.url}`).join('\n\n'); }
function applyHint() {
  if (latestSummary?.allow_apply) return 'Saved to stack config. Use Apply Xray to update the live service.';
  return 'Saved to stack config. Live apply is locked on this daemon, so Xray will not change until apply mode is enabled.';
}
async function runAction(fn: () => Promise<void>) {
  try { await fn(); } catch (error) { message(`Error: ${error instanceof Error ? error.message : String(error)}`); }
}

function render(summary: Summary, health: Health, plan: ApplyPlan, usage: Usage, quota: QuotaPlan, bandwidth: BandwidthPlan, system: SystemInfo, failover: FailoverDecision) {
  latestSummary = summary;
  const topUsers = [...usage.users].sort((a,b) => b.total - a.total).slice(0, 6);
  app.innerHTML = `
    <header class="shell-header">
      <div><p class="eyebrow">Xray Stack</p><h1>${summary.public_ip}</h1></div>
      <div class="actions"><button id="sync-usage">Sync usage</button><button id="refresh">Refresh</button><button id="apply" ${summary.allow_apply ? '' : 'disabled'}>Apply Xray</button></div>
    </header>
    <main class="shell">
      ${summary.allow_apply ? '' : '<section class="notice">Live apply is locked. Management changes are saved to stack config only until the daemon is started with apply mode.</section>'}
      <section class="metrics">
        <article><span>Users</span><strong>${summary.users}</strong></article>
        <article><span>SOCKS</span><strong>${summary.socks}</strong></article>
        <article><span>Failover</span><strong>${summary.failover.enabled ? 'On' : 'Off'}</strong></article>
        <article><span>Apply</span><strong>${summary.allow_apply ? 'Enabled' : 'Locked'}</strong></article>
        <article><span>Live sync</span><strong>${plan.changed ? 'Pending' : 'Synced'}</strong></article>
        <article><span>Quota actions</span><strong>${quota.actions.length}</strong></article>
        <article><span>Speed limits</span><strong>${bandwidth.limits.length}</strong></article>
      </section>
      <section class="tabs">
        <button data-tab="status" class="active">Status</button>
        <button data-tab="users">Users</button>
        <button data-tab="routes">Routes</button>
        <button data-tab="socks">SOCKS</button>
      </section>
      <section id="tab-status" class="tab-panel">${renderStatus(summary, health, plan, usage, quota, bandwidth, system, failover, topUsers)}</section>
      <section id="tab-users" class="tab-panel hidden">${renderUsers(summary)}</section>
      <section id="tab-routes" class="tab-panel hidden">${renderRoutes(summary)}</section>
      <section id="tab-socks" class="tab-panel hidden">${renderSocks(summary)}</section>
    </main>`;
  bindEvents();
}

function renderStatus(summary: Summary, health: Health, plan: ApplyPlan, usage: Usage, quota: QuotaPlan, bandwidth: BandwidthPlan, system: SystemInfo, failover: FailoverDecision, topUsers: Usage['users']) {
  const decision = failover.decision;
  return `
    <section class="metrics">
      <article><span>CPU</span><strong>${system.cpu.percent.toFixed(0)}%</strong><small>${esc(system.cpu.detail)}</small></article>
      <article><span>RAM</span><strong>${system.ram.percent.toFixed(0)}%</strong><small>${esc(system.ram.detail)}</small></article>
      ${system.tunnels.map(t => `<article><span>${esc(t.name)} traffic</span><strong>${bytes(t.rx_bytes + t.tx_bytes)}</strong><small>in ${bytes(t.rx_bytes)} · out ${bytes(t.tx_bytes)}</small></article>`).join('')}
    </section>
    <section class="grid2">
      <section class="panel">
        <div class="panel-head"><h2>Tunnels</h2><span>${new Date(health.generated_at).toLocaleString()}</span></div>
        <div class="tunnel-list">${health.tunnels.map(t => `<article class="tunnel"><div><h3>${esc(t.name)}</h3><p>${esc(t.interface)}${t.ipv4 ? ` · ${esc(t.ipv4)}` : ''}${t.probe ? ` · ${esc(t.probe)}` : ''}</p></div><div class="tunnel-state">${badge(t.up, t.up ? 'up' : 'down')}${badge(t.healthy, t.healthy ? `${t.latency_ms ?? 0}ms` : 'unhealthy')}</div>${t.error ? `<p class="error">${esc(t.error)}</p>` : ''}</article>`).join('')}</div>
      </section>
      <section class="panel">
        <div class="panel-head"><h2>Xray apply</h2>${badge(Boolean(plan.valid) && !plan.changed, plan.changed ? 'pending changes' : (plan.valid ? 'synced' : 'invalid'))}</div>
        <p class="muted">${esc(plan.config_path || '-')}</p>
        <p class="muted">${plan.allow_apply ? 'Live apply is enabled for this daemon.' : 'Live apply is locked. Start daemon with -allow-apply to enable writes.'}</p>
        ${plan.changed ? '<p class="warning">Stack config differs from the live Xray config.</p>' : ''}
        ${plan.error ? `<p class="error">${esc(plan.error)}</p>` : ''}
      </section>
    </section>
    <section class="panel">
      <div class="panel-head"><h2>Failover route</h2>${badge(!decision.pending, decision.pending ? `pending ${decision.confirmation_count}` : 'stable')}</div>
      <div class="route-grid">
        <article><span>Current</span><strong>${esc(formatMode(decision.current))}</strong></article>
        <article><span>Desired</span><strong>${esc(formatMode(decision.desired))}</strong></article>
        <article><span>Effective</span><strong>${esc(formatMode(decision.effective))}</strong></article>
      </div>
      <p class="muted">${esc(decision.reason)}</p>
    </section>
    <section class="panel">
      <div class="panel-head"><h2>Usage</h2><span>${usage.updated_at ? new Date(usage.updated_at * 1000).toLocaleString() : 'not synced'}</span></div>
      <div class="usage-list">${topUsers.map(u => `<article><strong>${esc(u.email)}</strong><span>${bytes(u.total)}</span><small>up ${bytes(u.uplink)} · down ${bytes(u.downlink)}</small></article>`).join('') || '<p class="muted">No usage yet.</p>'}</div>
    </section>
    <section class="grid2">
      <section class="panel">
        <div class="panel-head"><h2>Quota enforcement</h2>${badge(quota.actions.length === 0, quota.actions.length ? `${quota.actions.length} pending` : 'clear')}</div>
        ${quota.actions.length ? `<div class="table">${quota.actions.map(a => `<article><strong>${esc(a.email)}</strong><span>${bytes(a.used_bytes)} / ${bytes(a.quota_bytes)}</span><small>${esc(a.action)}</small></article>`).join('')}</div><button id="apply-quota" ${summary.allow_apply ? '' : 'disabled'}>Apply quota actions</button>` : '<p class="muted">No enabled user is over quota.</p>'}
      </section>
      <section class="panel">
        <div class="panel-head"><h2>Bandwidth limits</h2><span>${esc(bandwidth.device || 'eth0')}</span></div>
        ${bandwidth.limits.length ? `<div class="table">${bandwidth.limits.map(l => `<article><strong>${esc(l.email)}</strong><span>port ${l.port}</span><small>down ${l.download_mbps || 'none'} Mbps · up ${l.upload_mbps || 'none'} Mbps</small></article>`).join('')}</div>` : '<p class="muted">No per-user speed limits configured.</p>'}
        <button id="apply-bandwidth" ${summary.allow_apply ? '' : 'disabled'}>Apply bandwidth rules</button>
      </section>
    </section>`;
}

function formatMode(mode: FailoverMode) {
  if (!mode) return '-';
  return mode.interface ? `${mode.outbound_tag}:${mode.interface}` : mode.outbound_tag;
}

function renderUsers(summary: Summary) {
  return `
    <section class="grid2">
      <section class="panel">
        <div class="panel-head"><h2>Users</h2><button id="add-user">Add user</button></div>
        <div class="user-list">${summary.user_items.map(u => `
          <article class="row-card">
            <div><h3>${esc(u.email)}</h3><p>${u.uuid.slice(0, 8)}...${u.uuid.slice(-4)} · ${userStatus(u)}</p><p class="muted">quota ${u.quota_bytes ? bytes(u.quota_bytes) : 'none'} · speed ${u.download_mbps || 'none'}/${u.upload_mbps || 'none'} Mbps${u.bandwidth_port ? ` · port ${u.bandwidth_port}` : ''}</p></div>
            <div class="row-actions">
              <button data-view-user="${esc(u.email)}">View config</button>
              <button data-activity-user="${esc(u.email)}">Activity</button>
              <button data-edit-user="${esc(u.email)}">Edit</button>
              <button data-quota-user="${esc(u.email)}">Quota</button>
              <button data-speed-user="${esc(u.email)}">Speed</button>
              ${u.banned_until ? `<button data-unban-user="${esc(u.email)}">Unban</button>` : `<button data-ban-user="${esc(u.email)}">Temp ban</button>`}
              <button class="danger" data-delete-user="${esc(u.email)}">Delete</button>
            </div>
          </article>`).join('')}</div>
      </section>
      <section class="panel">
        <div class="panel-head"><h2>Connection config</h2><button id="copy-config">Copy</button></div>
        <textarea id="config-output" readonly placeholder="Select View config for a user."></textarea>
        <p id="action-msg" class="muted"></p>
      </section>
    </section>`;
}

function renderRoutes(summary: Summary) {
  return `
    <section class="grid2">
      <section class="panel">
        <div class="panel-head"><h2>Direct domains</h2><button id="add-direct">Add domain</button></div>
        <div class="table">${summary.direct_domains.map(d => `<article><strong>${esc(d)}</strong><button class="danger" data-delete-direct="${esc(d)}">Delete</button></article>`).join('') || '<p class="muted">No direct domains.</p>'}</div>
      </section>
      <section class="panel">
        <div class="panel-head"><h2>Blocked domains</h2><span>${summary.block_domains.length + summary.manual_blocks.length}</span></div>
        <div class="table">${[...summary.block_domains, ...summary.manual_blocks].map(d => `<article><strong>${esc(d)}</strong><small>blocked</small></article>`).join('')}</div>
      </section>
    </section>`;
}

function renderSocks(summary: Summary) {
  return `<section class="panel"><div class="panel-head"><h2>SOCKS users</h2><button id="add-socks">Add SOCKS</button></div><div class="user-list">${summary.socks_items.map(s => `<article class="row-card"><div><h3>${esc(s.username)}</h3><p>${esc(s.name)} · port ${s.port}</p></div><div class="row-actions"><button data-view-socks="${esc(s.username)}">View config</button><button data-edit-socks="${esc(s.username)}">Edit</button><button class="danger" data-delete-socks="${esc(s.username)}">Delete</button></div></article>`).join('')}</div></section>`;
}

function bindEvents() {
  document.querySelector('#refresh')?.addEventListener('click', load);
  document.querySelector('#apply')?.addEventListener('click', () => runAction(applyXray));
  document.querySelector('#sync-usage')?.addEventListener('click', () => runAction(syncUsage));
  document.querySelector('#apply-quota')?.addEventListener('click', () => runAction(applyQuota));
  document.querySelector('#apply-bandwidth')?.addEventListener('click', () => runAction(applyBandwidth));
  document.querySelectorAll<HTMLButtonElement>('[data-tab]').forEach(b => b.addEventListener('click', () => switchTab(b.dataset.tab!)));
  document.querySelector('#add-user')?.addEventListener('click', () => runAction(addUser));
  document.querySelector('#add-direct')?.addEventListener('click', () => runAction(addDirect));
  document.querySelector('#add-socks')?.addEventListener('click', () => runAction(addSocks));
  document.querySelector('#copy-config')?.addEventListener('click', () => runAction(copyConfig));
  document.querySelectorAll<HTMLButtonElement>('[data-view-user]').forEach(b => b.addEventListener('click', () => showUserConfig(b.dataset.viewUser!)));
  document.querySelectorAll<HTMLButtonElement>('[data-activity-user]').forEach(b => b.addEventListener('click', () => runAction(() => showActivity(b.dataset.activityUser!))));
  document.querySelectorAll<HTMLButtonElement>('[data-view-socks]').forEach(b => b.addEventListener('click', () => showSocksConfig(b.dataset.viewSocks!)));
  document.querySelectorAll<HTMLButtonElement>('[data-edit-socks]').forEach(b => b.addEventListener('click', () => runAction(() => editSocks(b.dataset.editSocks!))));
  document.querySelectorAll<HTMLButtonElement>('[data-delete-socks]').forEach(b => b.addEventListener('click', () => runAction(() => deleteSocks(b.dataset.deleteSocks!))));
  document.querySelectorAll<HTMLButtonElement>('[data-edit-user]').forEach(b => b.addEventListener('click', () => runAction(() => editUser(b.dataset.editUser!))));
  document.querySelectorAll<HTMLButtonElement>('[data-quota-user]').forEach(b => b.addEventListener('click', () => runAction(() => setQuota(b.dataset.quotaUser!))));
  document.querySelectorAll<HTMLButtonElement>('[data-speed-user]').forEach(b => b.addEventListener('click', () => runAction(() => setSpeed(b.dataset.speedUser!))));
  document.querySelectorAll<HTMLButtonElement>('[data-ban-user]').forEach(b => b.addEventListener('click', () => runAction(() => banUser(b.dataset.banUser!))));
  document.querySelectorAll<HTMLButtonElement>('[data-unban-user]').forEach(b => b.addEventListener('click', () => runAction(() => unbanUser(b.dataset.unbanUser!))));
  document.querySelectorAll<HTMLButtonElement>('[data-delete-user]').forEach(b => b.addEventListener('click', () => runAction(() => deleteUser(b.dataset.deleteUser!))));
  document.querySelectorAll<HTMLButtonElement>('[data-delete-direct]').forEach(b => b.addEventListener('click', () => runAction(() => deleteDirect(b.dataset.deleteDirect!))));
}

function switchTab(name: string) {
  document.querySelectorAll('[data-tab]').forEach(b => b.classList.toggle('active', (b as HTMLElement).dataset.tab === name));
  document.querySelectorAll('.tab-panel').forEach(p => p.classList.add('hidden'));
  document.querySelector(`#tab-${name}`)?.classList.remove('hidden');
}
function selectedUser(email: string) { return latestSummary?.user_items.find(u => u.email === email); }
function selectedSocks(user: string) { return latestSummary?.socks_items.find(s => s.username === user); }
function message(text: string) { const el = document.querySelector('#action-msg'); if (el) el.textContent = text; }
function output(text: string) { const el = document.querySelector<HTMLTextAreaElement>('#config-output'); if (el) el.value = text; }
function userStatus(u: UserItem) {
  if (u.banned_until) return `banned until ${new Date(u.banned_until * 1000).toLocaleString()}`;
  return u.enabled ? 'enabled' : 'disabled';
}

async function addUser() {
  const email = prompt('Email/name for new VLESS user');
  if (!email) return;
  const res = await post('/api/users', {email: email.trim(), uuid: ''}) as {links:Link[]};
  output(linkText(res.links || [])); message(`User added. ${applyHint()}`);
  await load(); switchTab('users'); output(linkText(res.links || []));
}
async function editUser(email: string) {
  const u = selectedUser(email); if (!u) return;
  const nextEmail = prompt('Email/name', u.email); if (!nextEmail) return;
  const uuid = prompt('UUID', u.uuid); if (!uuid) return;
  const enabled = confirm('Should this user be enabled? OK = enabled, Cancel = disabled');
  await put('/api/users', {old_email: u.email, email: nextEmail.trim(), uuid: uuid.trim(), enabled});
  await load(); switchTab('users'); message(`User updated. ${applyHint()}`);
}
async function deleteUser(email: string) {
  if (!confirm(`Delete ${email} from stack config?`)) return;
  await fetchJSON(`/api/users?email=${encodeURIComponent(email)}`, {method: 'DELETE'});
  await load(); switchTab('users'); message(`User deleted. ${applyHint()}`);
}
async function setQuota(email: string) {
  const u = selectedUser(email); if (!u) return;
  const current = u.quota_bytes ? String(Math.round(u.quota_bytes / 1024 / 1024 / 1024)) : '';
  const value = prompt('Traffic quota in GB. Empty = unlimited.', current);
  if (value === null) return;
  const gb = value.trim() ? Number(value) : 0;
  await post('/api/users/quota', {email, quota_bytes: Math.max(0, Math.round(gb * 1024 * 1024 * 1024))});
  await load(); switchTab('users'); message(`Quota updated. ${applyHint()}`);
}
async function setSpeed(email: string) {
  const u = selectedUser(email); if (!u) return;
  const down = prompt('Download Mbps. Empty = none.', u.download_mbps ? String(u.download_mbps) : '');
  if (down === null) return;
  const up = prompt('Upload Mbps. Empty = none.', u.upload_mbps ? String(u.upload_mbps) : '');
  if (up === null) return;
  await post('/api/users/bandwidth', {email, download_mbps: Number(down || 0), upload_mbps: Number(up || 0)});
  await load(); switchTab('users'); showUserConfig(email); message(`Speed limit updated. Use the limited config link after Xray and bandwidth rules are applied. ${applyHint()}`);
}
async function banUser(email: string) {
  const minutes = Number(prompt(`Temporary ban duration for ${email} in minutes.`, '60') || 0);
  if (!minutes) return;
  await post('/api/users/ban', {email, minutes});
  await load(); switchTab('users'); message(`User banned. ${applyHint()}`);
}
async function unbanUser(email: string) {
  await post('/api/users/unban', {email});
  await load(); switchTab('users'); message(`User unbanned. ${applyHint()}`);
}
function showUserConfig(email: string) { const u = selectedUser(email); if (u) output(linkText(u.links || [])); }
function showSocksConfig(username: string) { const s = selectedSocks(username); if (s) output(linkText(s.links || [])); }
async function showActivity(email: string) {
  const data = await fetchJSON<{items:Array<{time:string; client_ip:string; protocol:string; destination:string; outbound:string}>}>(`/api/users/activity?email=${encodeURIComponent(email)}`);
  output(data.items.length ? data.items.map(a => `${a.time}  ${a.client_ip}  ${a.protocol}:${a.destination}  ${a.outbound}`).join('\n') : 'No recent activity.');
  message(`Recent activity for ${email}`);
}
async function copyConfig() {
  const value = document.querySelector<HTMLTextAreaElement>('#config-output')?.value || '';
  await navigator.clipboard.writeText(value); message('Copied.');
}
async function addDirect() {
  const domain = prompt('Direct domain rule, e.g. domain:example.com or full:example.com');
  if (!domain) return;
  await post('/api/direct-domains', {domain: domain.trim()});
  await load(); switchTab('routes'); message(`Direct rule added. ${applyHint()}`);
}
async function addSocks() {
  const username = prompt('SOCKS username'); if (!username) return;
  const port = Number(prompt('Port', '1081') || 0); if (!port) return;
  const password = prompt('Password. Empty = auto-generate.', '') || '';
  await post('/api/socks', {name: username.trim(), listen: '0.0.0.0', port, username: username.trim(), password});
  await load(); switchTab('socks'); message(`SOCKS user added. ${applyHint()}`);
}
async function editSocks(username: string) {
  const s = selectedSocks(username); if (!s) return;
  const nextUser = prompt('SOCKS username', s.username); if (!nextUser) return;
  const nextName = prompt('Name/tag', s.name) || nextUser;
  const nextPort = Number(prompt('Port', String(s.port)) || 0); if (!nextPort) return;
  const nextPass = prompt('Password. Empty = keep current.', '') || '';
  await put('/api/socks', {old_username: s.username, name: nextName.trim(), listen: s.listen || '0.0.0.0', port: nextPort, username: nextUser.trim(), password: nextPass});
  await load(); switchTab('socks'); message(`SOCKS user updated. ${applyHint()}`);
}
async function deleteSocks(username: string) {
  if (!confirm(`Delete SOCKS user ${username}?`)) return;
  await fetchJSON(`/api/socks?username=${encodeURIComponent(username)}`, {method: 'DELETE'});
  await load(); switchTab('socks'); message(`SOCKS user deleted. ${applyHint()}`);
}
async function deleteDirect(domain: string) {
  await fetchJSON(`/api/direct-domains?domain=${encodeURIComponent(domain)}`, {method: 'DELETE'});
  await load(); switchTab('routes'); message(`Direct rule deleted. ${applyHint()}`);
}
async function applyXray() { if (confirm('Apply generated Xray config and restart Xray?')) { await fetchJSON('/api/xray/apply', {method: 'POST'}); await load(); } }
async function syncUsage() { await fetchJSON('/api/usage/sync', {method: 'POST'}); await load(); }
async function applyQuota() { if (confirm('Disable users that are over quota and apply Xray config?')) { await fetchJSON('/api/quota/apply', {method: 'POST'}); await load(); } }
async function applyBandwidth() { if (confirm('Apply nft/tc bandwidth rules on the server?')) { await fetchJSON('/api/bandwidth/apply', {method: 'POST'}); await load(); } }

async function load() {
  app.innerHTML = '<div class="loading">Loading...</div>';
  try {
    const [summary, health, plan, usage, quota, bandwidth, system, failover] = await Promise.all([
      fetchJSON<Summary>('/api/config/summary'),
      fetchJSON<Health>('/api/health'),
      fetchJSON<ApplyPlan>('/api/xray/apply-plan').catch(error => ({ok:false, valid:false, changed:false, config_path:'', allow_apply:false, error:String(error)})),
      fetchJSON<Usage>('/api/usage').catch(() => ({updated_at:0, users:[]})),
      fetchJSON<QuotaPlan>('/api/quota/plan').catch(() => ({generated_at:0, actions:[]})),
      fetchJSON<BandwidthPlan>('/api/bandwidth/plan').catch(() => ({device:'eth0', limits:[], needs_apply:false, apply_locked:true, tc_commands:[]})),
      fetchJSON<SystemInfo>('/api/system').catch(() => ({cpu:{percent:0, detail:'unavailable'}, ram:{percent:0, detail:'unavailable', used_bytes:0, total_bytes:0}, tunnels:[], updated_at:0})),
      fetchJSON<FailoverDecision>('/api/failover/decision').catch(() => ({decision:{current:{outbound_tag:'unknown'}, desired:{outbound_tag:'unknown'}, effective:{outbound_tag:'unknown'}, pending:false, confirmation_count:0, reason:'unavailable'}})),
    ]);
    render(summary, health, plan, usage, quota, bandwidth, system, failover);
  } catch (error) { app.innerHTML = `<div class="error-page"><h1>Load failed</h1><pre>${esc(String(error))}</pre></div>`; }
}
void load();
