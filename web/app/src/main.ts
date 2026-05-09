import './styles.css';

type Summary = {
  public_ip: string;
  users: number;
  socks: number;
  allow_apply: boolean;
  tunnels: Array<{name:string; type:string; interface:string; priority:number}>;
  failover: {enabled:boolean; probe_ip:string; probe_port:number; cooldown_seconds:number};
};

type Health = { ok: boolean; generated_at: string; tunnels: Array<{name:string; interface:string; up:boolean; healthy:boolean; ipv4?:string; latency_ms?:number; error?:string}> };
type ApplyPlan = { ok: boolean; valid: boolean; config_path: string; allow_apply: boolean; error?: string };
type Usage = { updated_at: number; users: Array<{email:string; uplink:number; downlink:number; total:number}> };
type QuotaPlan = { generated_at: number; actions: Array<{email:string; used_bytes:number; quota_bytes:number; action:string; reason:string}> };
type BandwidthPlan = { device: string; limits: Array<{email:string; port:number; download_mbps:number; upload_mbps:number}>; needs_apply: boolean; apply_locked: boolean; tc_commands: string[] };

const apiBase = import.meta.env.VITE_API_BASE || '';
const app = document.querySelector<HTMLDivElement>('#app')!;

async function fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiBase}${path}`, {cache: 'no-store', ...init});
  const body = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body.error || `${response.status} ${response.statusText}`);
  return body;
}

function badge(ok: boolean, label: string) { return `<span class="badge ${ok ? 'ok' : 'bad'}"><span></span>${label}</span>`; }
function bytes(value: number) {
  const units = ['B','KB','MB','GB','TB']; let n = value || 0; let i = 0;
  while (n >= 1024 && i < units.length - 1) { n /= 1024; i++; }
  return `${n.toFixed(i ? 1 : 0)} ${units[i]}`;
}

function render(summary: Summary, health: Health, plan: ApplyPlan, usage: Usage, quota: QuotaPlan, bandwidth: BandwidthPlan) {
  const topUsers = [...usage.users].sort((a,b) => b.total - a.total).slice(0, 6);
  app.innerHTML = `
    <header class="shell-header">
      <div><p class="eyebrow">Xray Stack</p><h1>${summary.public_ip}</h1></div>
      <div class="actions"><button id="sync-usage">Sync usage</button><button id="refresh">Refresh</button><button id="apply" ${summary.allow_apply ? '' : 'disabled'}>Apply Xray</button></div>
    </header>
    <main class="shell">
      <section class="metrics">
        <article><span>Users</span><strong>${summary.users}</strong></article>
        <article><span>SOCKS</span><strong>${summary.socks}</strong></article>
        <article><span>Failover</span><strong>${summary.failover.enabled ? 'On' : 'Off'}</strong></article>
        <article><span>Apply</span><strong>${summary.allow_apply ? 'Enabled' : 'Locked'}</strong></article>
        <article><span>Quota actions</span><strong>${quota.actions.length}</strong></article>
        <article><span>Speed limits</span><strong>${bandwidth.limits.length}</strong></article>
      </section>
      <section class="grid2">
        <section class="panel">
          <div class="panel-head"><h2>Tunnels</h2><span>${new Date(health.generated_at).toLocaleString()}</span></div>
          <div class="tunnel-list">${health.tunnels.map(t => `<article class="tunnel"><div><h3>${t.name}</h3><p>${t.interface}${t.ipv4 ? ` · ${t.ipv4}` : ''}</p></div><div class="tunnel-state">${badge(t.up, t.up ? 'up' : 'down')}${badge(t.healthy, t.healthy ? `${t.latency_ms ?? 0}ms` : 'unhealthy')}</div>${t.error ? `<p class="error">${t.error}</p>` : ''}</article>`).join('')}</div>
        </section>
        <section class="panel">
          <div class="panel-head"><h2>Xray apply</h2>${badge(Boolean(plan.valid), plan.valid ? 'valid' : 'invalid')}</div>
          <p class="muted">${plan.config_path || '-'}</p>
          <p class="muted">${plan.allow_apply ? 'Live apply is enabled for this daemon.' : 'Live apply is locked. Start daemon with -allow-apply to enable writes.'}</p>
          ${plan.error ? `<p class="error">${plan.error}</p>` : ''}
        </section>
      </section>
      <section class="panel">
        <div class="panel-head"><h2>Usage</h2><span>${usage.updated_at ? new Date(usage.updated_at * 1000).toLocaleString() : 'not synced'}</span></div>
        <div class="usage-list">${topUsers.map(u => `<article><strong>${u.email}</strong><span>${bytes(u.total)}</span><small>up ${bytes(u.uplink)} · down ${bytes(u.downlink)}</small></article>`).join('') || '<p class="muted">No usage yet.</p>'}</div>
      </section>
      <section class="grid2">
        <section class="panel">
          <div class="panel-head"><h2>Quota enforcement</h2>${badge(quota.actions.length === 0, quota.actions.length ? `${quota.actions.length} pending` : 'clear')}</div>
          ${quota.actions.length ? `<div class="table">${quota.actions.map(a => `<article><strong>${a.email}</strong><span>${bytes(a.used_bytes)} / ${bytes(a.quota_bytes)}</span><small>${a.action}</small></article>`).join('')}</div><button id="apply-quota" ${summary.allow_apply ? '' : 'disabled'}>Apply quota actions</button>` : '<p class="muted">No enabled user is over quota.</p>'}
        </section>
        <section class="panel">
          <div class="panel-head"><h2>Bandwidth limits</h2><span>${bandwidth.device || 'eth0'}</span></div>
          ${bandwidth.limits.length ? `<div class="table">${bandwidth.limits.map(l => `<article><strong>${l.email}</strong><span>port ${l.port}</span><small>down ${l.download_mbps || 'none'} Mbps · up ${l.upload_mbps || 'none'} Mbps</small></article>`).join('')}</div>` : '<p class="muted">No per-user speed limits configured.</p>'}
          <button id="apply-bandwidth" ${summary.allow_apply ? '' : 'disabled'}>Apply bandwidth rules</button>
        </section>
      </section>
    </main>`;
  document.querySelector('#refresh')?.addEventListener('click', load);
  document.querySelector('#apply')?.addEventListener('click', applyXray);
  document.querySelector('#sync-usage')?.addEventListener('click', syncUsage);
  document.querySelector('#apply-quota')?.addEventListener('click', applyQuota);
  document.querySelector('#apply-bandwidth')?.addEventListener('click', applyBandwidth);
}

async function applyXray() {
  if (!confirm('Apply generated Xray config and restart Xray?')) return;
  await fetchJSON('/api/xray/apply', {method: 'POST'});
  await load();
}

async function syncUsage() {
  await fetchJSON('/api/usage/sync', {method: 'POST'});
  await load();
}

async function applyQuota() {
  if (!confirm('Disable users that are over quota and apply Xray config?')) return;
  await fetchJSON('/api/quota/apply', {method: 'POST'});
  await load();
}

async function applyBandwidth() {
  if (!confirm('Apply nft/tc bandwidth rules on the server?')) return;
  await fetchJSON('/api/bandwidth/apply', {method: 'POST'});
  await load();
}

async function load() {
  app.innerHTML = '<div class="loading">Loading...</div>';
  try {
    const [summary, health, plan, usage, quota, bandwidth] = await Promise.all([
      fetchJSON<Summary>('/api/config/summary'),
      fetchJSON<Health>('/api/health'),
      fetchJSON<ApplyPlan>('/api/xray/apply-plan').catch(error => ({ok:false, valid:false, config_path:'', allow_apply:false, error:String(error)})),
      fetchJSON<Usage>('/api/usage').catch(() => ({updated_at:0, users:[]})),
      fetchJSON<QuotaPlan>('/api/quota/plan').catch(() => ({generated_at:0, actions:[]})),
      fetchJSON<BandwidthPlan>('/api/bandwidth/plan').catch(() => ({device:'eth0', limits:[], needs_apply:false, apply_locked:true, tc_commands:[]})),
    ]);
    render(summary, health, plan, usage, quota, bandwidth);
  } catch (error) { app.innerHTML = `<div class="error-page"><h1>Load failed</h1><pre>${String(error)}</pre></div>`; }
}

void load();
