import './styles.css';

type Summary = {
  public_ip: string;
  users: number;
  socks: number;
  tunnels: Array<{name:string; type:string; interface:string; priority:number}>;
  failover: {enabled:boolean; probe_ip:string; probe_port:number; cooldown_seconds:number};
};

type Health = {
  ok: boolean;
  generated_at: string;
  tunnels: Array<{name:string; interface:string; up:boolean; healthy:boolean; ipv4?:string; latency_ms?:number; error?:string}>;
};

const apiBase = import.meta.env.VITE_API_BASE || '';
const app = document.querySelector<HTMLDivElement>('#app')!;

async function fetchJSON<T>(path: string): Promise<T> {
  const response = await fetch(`${apiBase}${path}`, {cache: 'no-store'});
  if (!response.ok) throw new Error(`${response.status} ${response.statusText}`);
  return response.json();
}

function badge(ok: boolean, label: string) {
  return `<span class="badge ${ok ? 'ok' : 'bad'}"><span></span>${label}</span>`;
}

function render(summary: Summary, health: Health) {
  app.innerHTML = `
    <header class="shell-header">
      <div>
        <p class="eyebrow">Xray Stack</p>
        <h1>${summary.public_ip}</h1>
      </div>
      <button id="refresh">Refresh</button>
    </header>
    <main class="shell">
      <section class="metrics">
        <article><span>Users</span><strong>${summary.users}</strong></article>
        <article><span>SOCKS</span><strong>${summary.socks}</strong></article>
        <article><span>Failover</span><strong>${summary.failover.enabled ? 'On' : 'Off'}</strong></article>
        <article><span>Probe</span><strong>${summary.failover.probe_ip}:${summary.failover.probe_port}</strong></article>
      </section>
      <section class="panel">
        <div class="panel-head">
          <h2>Tunnels</h2>
          <span>${new Date(health.generated_at).toLocaleString()}</span>
        </div>
        <div class="tunnel-list">
          ${health.tunnels.map(t => `
            <article class="tunnel">
              <div>
                <h3>${t.name}</h3>
                <p>${t.interface}${t.ipv4 ? ` · ${t.ipv4}` : ''}</p>
              </div>
              <div class="tunnel-state">
                ${badge(t.up, t.up ? 'up' : 'down')}
                ${badge(t.healthy, t.healthy ? `${t.latency_ms ?? 0}ms` : 'unhealthy')}
              </div>
              ${t.error ? `<p class="error">${t.error}</p>` : ''}
            </article>`).join('')}
        </div>
      </section>
    </main>`;
  document.querySelector('#refresh')?.addEventListener('click', load);
}

async function load() {
  app.innerHTML = '<div class="loading">Loading...</div>';
  try {
    const [summary, health] = await Promise.all([
      fetchJSON<Summary>('/api/config/summary'),
      fetchJSON<Health>('/api/health'),
    ]);
    render(summary, health);
  } catch (error) {
    app.innerHTML = `<div class="error-page"><h1>Load failed</h1><pre>${String(error)}</pre></div>`;
  }
}

void load();
