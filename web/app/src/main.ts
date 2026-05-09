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

function render(summary: Summary, health: Health, plan: ApplyPlan, usage: Usage) {
  const topUsers = [...usage.users].sort((a,b) => b.total - a.total).slice(0, 6);
  app.innerHTML = `
    <header class="shell-header">
      <div><p class="eyebrow">Xray Stack</p><h1>${summary.public_ip}</h1></div>
      <div class="actions"><button id="refresh">Refresh</button><button id="apply" ${summary.allow_apply ? '' : 'disabled'}>Apply Xray</button></div>
    </header>
    <main class="shell">
      <section class="metrics">
        <article><span>Users</span><strong>${summary.users}</strong></article>
        <article><span>SOCKS</span><strong>${summary.socks}</strong></article>
        <article><span>Failover</span><strong>${summary.failover.enabled ? 'On' : 'Off'}</strong></article>
        <article><span>Apply</span><strong>${summary.allow_apply ? 'Enabled' : 'Locked'}</strong></article>
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
    </main>`;
  document.querySelector('#refresh')?.addEventListener('click', load);
  document.querySelector('#apply')?.addEventListener('click', applyXray);
}

async function applyXray() {
  if (!confirm('Apply generated Xray config and restart Xray?')) return;
  await fetchJSON('/api/xray/apply', {method: 'POST'});
  await load();
}

async function load() {
  app.innerHTML = '<div class="loading">Loading...</div>';
  try {
    const [summary, health, plan, usage] = await Promise.all([
      fetchJSON<Summary>('/api/config/summary'),
      fetchJSON<Health>('/api/health'),
      fetchJSON<ApplyPlan>('/api/xray/apply-plan').catch(error => ({ok:false, valid:false, config_path:'', allow_apply:false, error:String(error)})),
      fetchJSON<Usage>('/api/usage').catch(() => ({updated_at:0, users:[]})),
    ]);
    render(summary, health, plan, usage);
  } catch (error) { app.innerHTML = `<div class="error-page"><h1>Load failed</h1><pre>${String(error)}</pre></div>`; }
}

void load();
