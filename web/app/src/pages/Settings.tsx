import { useEffect, useState } from 'react';
import { Bell, Copy, Globe2, KeyRound, MessageCircle, Plus, RefreshCw, Save, Send, ShieldCheck, Trash2, UserCog } from 'lucide-react';
import PageHeader from '../components/PageHeader';
import { useClientEndpointHealth, useNotifications, useSummary, useTokens } from '../api/hooks';
import { useAdmins, useMe } from '../api/auth';
import { post, del, put, api } from '../api/client';
import { useQueryClient } from '@tanstack/react-query';
import { useToast } from '../components/Toast';
import { formatTime, relativeTime } from '../lib/format';
import { copyText } from '../lib/clipboard';
import type { ClientEndpoint, ClientEndpointHealth } from '../api/types';

type TelegramChat = { chat_id: number; title: string; type: string; from?: string };

const EVENT_KINDS = ['apply', 'audit', 'failover', 'tunnel', 'quota', 'test'];

export default function Settings() {
  const { data: summary } = useSummary();
  const failover = summary?.failover;

  return (
    <>
      <PageHeader title="Settings" subtitle="Apply mode, failover, tokens, notifications." />
      <div className="grid lg:grid-cols-2 gap-5">
        <div className="panel panel-pad">
          <h2 className="text-sm font-semibold tracking-tight mb-3">Apply mode</h2>
          <p className="text-sm text-muted dark:text-muted-dark mb-3">Daemon flag <code className="font-mono">-allow-apply</code> controls whether the panel can mutate live Xray.</p>
          <div className={`pill ${summary?.allow_apply ? 'pill-ok' : 'pill-bad'}`}><span className="dot" />{summary?.allow_apply ? 'Apply enabled' : 'Apply locked'}</div>
        </div>

        <div className="panel panel-pad">
          <h2 className="text-sm font-semibold tracking-tight mb-3">Failover</h2>
          {failover ? (
            <dl className="text-sm grid grid-cols-2 gap-3">
              <div><dt className="kpi-label">Enabled</dt><dd>{failover.enabled ? 'Yes' : 'No'}</dd></div>
              <div><dt className="kpi-label">Probe</dt><dd className="font-mono text-xs">{failover.probe_ip}:{failover.probe_port}</dd></div>
              <div><dt className="kpi-label">Cooldown</dt><dd>{failover.cooldown_seconds}s</dd></div>
            </dl>
          ) : <div className="text-sm text-muted dark:text-muted-dark">—</div>}
        </div>

        <ClientEndpointsPanel />
        <AdminsPanel />
        <TokensPanel />
        <NotificationsPanel />
      </div>
    </>
  );
}

function AdminsPanel() {
  const me = useMe();
  const admins = useAdmins();
  const qc = useQueryClient();
  const toast = useToast();
  const [creating, setCreating] = useState(false);
  const [pwTarget, setPwTarget] = useState<string | null>(null);

  async function remove(username: string) {
    if (!confirm(`Remove admin ${username}?`)) return;
    try {
      await del(`/api/admins?username=${encodeURIComponent(username)}`);
      toast.show('Admin removed', 'ok');
      qc.invalidateQueries({ queryKey: ['admins'] });
    } catch (e: any) {
      toast.show(`Remove failed: ${e?.message}`, 'bad');
    }
  }

  const items = admins.data?.admins ?? [];
  return (
    <div className="panel">
      <div className="flex items-center justify-between px-5 py-3 border-b border-border dark:border-border-dark">
        <h2 className="text-sm font-semibold tracking-tight flex items-center gap-2"><ShieldCheck size={14} /> Panel admins</h2>
        <button className="btn text-xs" onClick={() => setCreating(true)}><Plus size={12} /> Add admin</button>
      </div>
      <div className="px-5 py-2 text-xs text-muted dark:text-muted-dark border-b border-border dark:border-border-dark">
        Admins sign in via the panel login page. Their password is hashed with PBKDF2-SHA256.
        Bearer tokens (above) still work for CLI / automation.
        {me.data?.bootstrap_needed && <> No admins exist yet — create the first one to enable the login page.</>}
      </div>
      <div className="divide-y divide-border dark:divide-border-dark">
        {items.length === 0 && (
          <div className="px-5 py-4 text-sm text-muted dark:text-muted-dark">No admins yet.</div>
        )}
        {items.map((a) => (
          <div key={a.username} className="px-5 py-3 flex items-center gap-3 text-sm">
            <UserCog size={14} className="text-muted" />
            <div className="flex-1 min-w-0">
              <div className="font-medium truncate">{a.username}</div>
              <div className="text-xs text-muted dark:text-muted-dark">
                {a.last_login ? <>last login {relativeTime(a.last_login)}</> : 'never signed in'}
                {a.created_at ? <> · created {relativeTime(a.created_at)}</> : null}
              </div>
            </div>
            <button className="btn text-xs" onClick={() => setPwTarget(a.username)}>Change password</button>
            <button className="btn btn-danger text-xs" onClick={() => remove(a.username)} disabled={items.length <= 1}>
              <Trash2 size={12} />
            </button>
          </div>
        ))}
      </div>
      {creating && (
        <AdminFormDialog
          title="Add admin"
          submitLabel="Create"
          requireUsername
          onCancel={() => setCreating(false)}
          onSubmit={async (username, password) => {
            try {
              await post('/api/admins', { username, password });
              toast.show('Admin created', 'ok');
              qc.invalidateQueries({ queryKey: ['admins'] });
              qc.invalidateQueries({ queryKey: ['me'] });
              setCreating(false);
            } catch (e: any) {
              toast.show(`Create failed: ${e?.message}`, 'bad');
            }
          }}
        />
      )}
      {pwTarget && (
        <AdminFormDialog
          title={`Change password — ${pwTarget}`}
          submitLabel="Save"
          fixedUsername={pwTarget}
          onCancel={() => setPwTarget(null)}
          onSubmit={async (username, password) => {
            try {
              await post('/api/admins/password', { username, password });
              toast.show('Password updated', 'ok');
              setPwTarget(null);
            } catch (e: any) {
              toast.show(`Update failed: ${e?.message}`, 'bad');
            }
          }}
        />
      )}
    </div>
  );
}

function AdminFormDialog({
  title,
  submitLabel,
  fixedUsername,
  requireUsername,
  onSubmit,
  onCancel,
}: {
  title: string;
  submitLabel: string;
  fixedUsername?: string;
  requireUsername?: boolean;
  onSubmit: (username: string, password: string) => Promise<void> | void;
  onCancel: () => void;
}) {
  const [username, setUsername] = useState(fixedUsername ?? '');
  const [password, setPassword] = useState('');
  const [pending, setPending] = useState(false);
  return (
    <div className="fixed inset-0 z-40 bg-black/40 grid place-items-center p-4" onClick={onCancel}>
      <form
        className="panel panel-pad w-full max-w-md"
        onClick={(e) => e.stopPropagation()}
        onSubmit={async (e) => {
          e.preventDefault();
          setPending(true);
          try {
            await onSubmit(username.trim(), password);
          } finally {
            setPending(false);
          }
        }}
      >
        <h3 className="text-base font-semibold mb-3 flex items-center gap-2"><ShieldCheck size={16} /> {title}</h3>
        {!fixedUsername && (
          <>
            <label className="kpi-label">Username</label>
            <input
              className="input mb-3"
              autoFocus
              required={requireUsername}
              value={username}
              onChange={(e) => setUsername(e.target.value)}
            />
          </>
        )}
        <label className="kpi-label">Password <span className="text-muted">(min 8 chars)</span></label>
        <input
          className="input mb-4"
          type="password"
          required
          minLength={8}
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        <div className="flex justify-end gap-2">
          <button type="button" className="btn" onClick={onCancel}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={pending || password.length < 8 || (!fixedUsername && !username)}>
            {submitLabel}
          </button>
        </div>
      </form>
    </div>
  );
}

function ClientEndpointsPanel() {
  const { data: summary } = useSummary();
  const health = useClientEndpointHealth();
  const qc = useQueryClient();
  const toast = useToast();
  const [form, setForm] = useState<ClientEndpoint>({
    name: 'pars-pack',
    host: 'edge.example.com',
    port: 443,
    network: 'xhttp',
    path: '/api/v1/events',
    mode: 'stream-up',
    tls: true,
    enabled: true,
  });
  const endpoints = summary?.client_endpoints ?? [];
  const healthByName = new Map((health.data?.endpoints ?? []).map((item) => [item.name, item]));

  function edit(ep: ClientEndpoint) {
    setForm({ ...ep });
  }

  async function save(e: React.FormEvent) {
    e.preventDefault();
    try {
      await put('/api/client-endpoints', form);
      toast.show('Client endpoint saved', 'ok');
      qc.invalidateQueries({ queryKey: ['summary'] });
    } catch (err: any) {
      toast.show(`Save failed: ${err?.message}`, 'bad');
    }
  }

  async function remove(name: string) {
    if (!confirm(`Delete endpoint ${name}?`)) return;
    try {
      await del(`/api/client-endpoints?name=${encodeURIComponent(name)}`);
      toast.show('Client endpoint deleted', 'ok');
      qc.invalidateQueries({ queryKey: ['summary'] });
    } catch (err: any) {
      toast.show(`Delete failed: ${err?.message}`, 'bad');
    }
  }

  return (
    <div className="panel lg:col-span-2">
      <div className="flex items-center justify-between px-5 py-3 border-b border-border dark:border-border-dark">
        <h2 className="text-sm font-semibold tracking-tight flex items-center gap-2"><Globe2 size={14} /> Client domains</h2>
        <button className="btn text-xs" onClick={() => health.refetch()} disabled={health.isFetching}>
          <RefreshCw size={12} className={health.isFetching ? 'animate-spin' : ''} /> Check
        </button>
      </div>
      <div className="grid lg:grid-cols-[1.1fr,1.4fr] gap-0">
        <form className="p-5 border-b lg:border-b-0 lg:border-r border-border dark:border-border-dark space-y-3" onSubmit={save}>
          <div className="grid sm:grid-cols-2 gap-2">
            <div>
              <label className="kpi-label">Name</label>
              <input className="input" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
            </div>
            <div>
              <label className="kpi-label">Host</label>
              <input className="input" value={form.host} onChange={(e) => setForm({ ...form, host: e.target.value })} required />
            </div>
          </div>
          <div className="grid grid-cols-[1fr,1fr,1fr] gap-2">
            <div>
              <label className="kpi-label">Port</label>
              <input className="input" type="number" min="1" max="65535" value={form.port} onChange={(e) => setForm({ ...form, port: parseInt(e.target.value, 10) || 443 })} required />
            </div>
            <div>
              <label className="kpi-label">Network</label>
              <select className="input" value={form.network} onChange={(e) => setForm({ ...form, network: e.target.value as 'ws' | 'xhttp' })}>
                <option value="ws">WS</option>
                <option value="xhttp">XHTTP</option>
              </select>
            </div>
            <div>
              <label className="kpi-label">TLS</label>
              <button type="button" className={`btn w-full justify-center ${form.tls ? 'btn-primary' : ''}`} onClick={() => setForm({ ...form, tls: !form.tls })}>
                {form.tls ? 'On' : 'Off'}
              </button>
            </div>
          </div>
          <div>
            <label className="kpi-label">Path</label>
            <input className="input" value={form.path} onChange={(e) => setForm({ ...form, path: e.target.value })} required />
          </div>
          {form.network === 'xhttp' && (
            <div>
              <label className="kpi-label">Mode</label>
              <select className="input" value={form.mode ?? 'stream-up'} onChange={(e) => setForm({ ...form, mode: e.target.value as ClientEndpoint['mode'] })}>
                <option value="stream-up">stream-up</option>
                <option value="packet-up">packet-up</option>
                <option value="stream-one">stream-one</option>
                <option value="auto">auto</option>
              </select>
            </div>
          )}
          <label className="inline-flex items-center gap-2 text-sm">
            <input type="checkbox" checked={form.enabled} onChange={(e) => setForm({ ...form, enabled: e.target.checked })} />
            Enabled
          </label>
          <div className="flex justify-end">
            <button className="btn btn-primary" type="submit"><Save size={12} /> Save endpoint</button>
          </div>
        </form>
        <div className="divide-y divide-border dark:divide-border-dark">
          {endpoints.length === 0 && (
            <div className="px-5 py-6 text-sm text-muted dark:text-muted-dark">No client domains configured.</div>
          )}
          {endpoints.map((ep) => (
            <div key={ep.name} className="px-5 py-3 grid sm:grid-cols-[1fr,1.6fr,auto] gap-3 items-center text-sm">
              <div>
                <div className="font-medium">{ep.name}</div>
                <EndpointStatus health={healthByName.get(ep.name)} enabled={ep.enabled} />
              </div>
              <div className="font-mono text-xs break-all">
                {ep.tls ? 'https' : 'http'}://{ep.host}:{ep.port}{ep.path} · {ep.network.toUpperCase()}{ep.mode ? `/${ep.mode}` : ''}
                {healthByName.get(ep.name) && (
                  <div className="font-sans text-xs text-muted dark:text-muted-dark mt-1">
                    path HTTP {healthByName.get(ep.name)?.status_code || '—'} · landing HTTP {healthByName.get(ep.name)?.landing_status || '—'} · {healthByName.get(ep.name)?.latency_ms ?? 0}ms
                  </div>
                )}
              </div>
              <div className="flex justify-end gap-1">
                <button className="btn px-2 py-1" onClick={() => edit(ep)} title="Edit"><Globe2 size={12} /></button>
                <button className="btn btn-danger px-2 py-1" onClick={() => remove(ep.name)} title="Delete"><Trash2 size={12} /></button>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function EndpointStatus({ health, enabled }: { health?: ClientEndpointHealth; enabled: boolean }) {
  if (!enabled) {
    return <div className="pill mt-1 text-muted"><span className="dot" />Disabled</div>;
  }
  if (!health) {
    return <div className="pill mt-1 text-muted"><span className="dot" />Unchecked</div>;
  }
  if (health.ok && health.landing_ok) {
    return <div className="pill mt-1 pill-ok"><span className="dot" />Healthy</div>;
  }
  if (health.ok) {
    return <div className="pill mt-1 pill-warn"><span className="dot" />Path ok</div>;
  }
  return <div className="pill mt-1 pill-bad" title={health.error || 'Endpoint failed'}><span className="dot" />Down</div>;
}

function TokensPanel() {
  const { data } = useTokens();
  const qc = useQueryClient();
  const toast = useToast();
  const [creating, setCreating] = useState(false);
  const [newPlain, setNewPlain] = useState<{ id: string; token: string } | null>(null);

  async function createToken(id: string, scope: string) {
    try {
      const res = await post<{ ok: boolean; id: string; token: string }>('/api/tokens', { id, scope });
      setNewPlain({ id: res.id, token: res.token });
      toast.show('Token created — copy it now', 'warn');
      qc.invalidateQueries({ queryKey: ['tokens'] });
    } catch (e: any) {
      toast.show(`Create failed: ${e?.message}`, 'bad');
    }
  }

  async function removeToken(id: string) {
    if (!confirm(`Revoke token ${id}? This cannot be undone.`)) return;
    try {
      await del(`/api/tokens?id=${encodeURIComponent(id)}`);
      toast.show('Token revoked', 'ok');
      qc.invalidateQueries({ queryKey: ['tokens'] });
    } catch (e: any) {
      toast.show(`Revoke failed: ${e?.message}`, 'bad');
    }
  }

  return (
    <div className="panel">
      <div className="flex items-center justify-between px-5 py-3 border-b border-border dark:border-border-dark">
        <h2 className="text-sm font-semibold tracking-tight flex items-center gap-2"><KeyRound size={14} /> API tokens</h2>
        <button className="btn btn-primary text-xs" onClick={() => setCreating(true)}><Plus size={12} /> New token</button>
      </div>
      <div className="divide-y divide-border dark:divide-border-dark">
        {(data?.tokens ?? []).length === 0 && (
          <div className="px-5 py-6 text-sm text-muted dark:text-muted-dark">No tokens. The panel still uses nginx Basic Auth at <code className="font-mono">/monitor/</code>.</div>
        )}
        {(data?.tokens ?? []).map((t) => (
          <div key={t.id} className="px-5 py-3 grid grid-cols-[1fr,1fr,auto] gap-3 text-sm items-center">
            <div>
              <div className="font-medium">{t.id}</div>
              <div className="text-xs text-muted dark:text-muted-dark font-mono">{t.hash_short}…</div>
            </div>
            <div className="text-xs text-muted dark:text-muted-dark">
              <div>{t.scope || 'read+write'}</div>
              <div>{t.last_used ? `Last used ${relativeTime(t.last_used)}` : 'Never used'} · created {formatTime(t.created_at)}</div>
            </div>
            <button className="btn btn-danger text-xs" onClick={() => removeToken(t.id)}><Trash2 size={12} /> Revoke</button>
          </div>
        ))}
      </div>
      {creating && <CreateTokenDialog onCancel={() => setCreating(false)} onCreate={(id, scope) => { createToken(id, scope); setCreating(false); }} />}
      {newPlain && <RevealTokenDialog id={newPlain.id} token={newPlain.token} onClose={() => setNewPlain(null)} />}
    </div>
  );
}

function CreateTokenDialog({ onCancel, onCreate }: { onCancel: () => void; onCreate: (id: string, scope: string) => void }) {
  const [id, setId] = useState('');
  const [scope, setScope] = useState('read+write');
  return (
    <div className="fixed inset-0 z-40 bg-black/40 grid place-items-center p-4" onClick={onCancel}>
      <form
        onClick={(e) => e.stopPropagation()}
        onSubmit={(e) => { e.preventDefault(); onCreate(id, scope); }}
        className="panel panel-pad w-full max-w-md"
      >
        <h3 className="text-base font-semibold mb-3 flex items-center gap-2"><KeyRound size={16} /> New API token</h3>
        <label className="kpi-label">ID</label>
        <input className="input mb-3" placeholder="ci-deploy" value={id} onChange={(e) => setId(e.target.value)} required autoFocus />
        <label className="kpi-label">Scope</label>
        <input className="input mb-4" placeholder="read+write" value={scope} onChange={(e) => setScope(e.target.value)} />
        <div className="flex justify-end gap-2">
          <button type="button" className="btn" onClick={onCancel}>Cancel</button>
          <button type="submit" className="btn btn-primary">Create</button>
        </div>
      </form>
    </div>
  );
}

function RevealTokenDialog({ id, token, onClose }: { id: string; token: string; onClose: () => void }) {
  const toast = useToast();
  return (
    <div className="fixed inset-0 z-40 bg-black/40 grid place-items-center p-4" onClick={onClose}>
      <div className="panel panel-pad w-full max-w-lg" onClick={(e) => e.stopPropagation()}>
        <h3 className="text-base font-semibold mb-2">Token <code className="font-mono">{id}</code> created</h3>
        <p className="text-xs text-warn dark:text-warn-dark mb-3">Save this token now — it cannot be retrieved later.</p>
        <div className="flex items-center gap-2 rounded-lg border border-border dark:border-border-dark p-3 mb-4">
          <code className="font-mono text-xs break-all flex-1">{token}</code>
          <button className="btn text-xs" onClick={async () => { const ok = await copyText(token); toast.show(ok ? 'Copied' : 'Copy failed — select text manually', ok ? 'ok' : 'bad'); }}><Copy size={12} /></button>
        </div>
        <div className="flex justify-end">
          <button className="btn btn-primary" onClick={onClose}>I saved it</button>
        </div>
      </div>
    </div>
  );
}

function NotificationsPanel() {
  const { data, refetch } = useNotifications();
  const toast = useToast();
  const [webhookUrl, setWebhookUrl] = useState('');
  const [webhookSecret, setWebhookSecret] = useState('');
  const [webhookEvents, setWebhookEvents] = useState<string[]>([]);
  const [tgBotToken, setTgBotToken] = useState('');
  const [tgChatID, setTgChatID] = useState('');
  const [tgEvents, setTgEvents] = useState<string[]>([]);
  const [pending, setPending] = useState(false);

  useEffect(() => {
    if (!data) return;
    setWebhookUrl(data.notifications.webhook.url ?? '');
    setWebhookEvents(data.notifications.webhook.events ?? []);
    setTgChatID(data.notifications.telegram.chat_id ?? '');
    setTgEvents(data.notifications.telegram.events ?? []);
  }, [data]);

  function toggle(set: string[], setter: (s: string[]) => void, kind: string) {
    setter(set.includes(kind) ? set.filter((k) => k !== kind) : [...set, kind]);
  }

  async function save() {
    setPending(true);
    try {
      await put('/api/notifications', {
        webhook: { url: webhookUrl, secret: webhookSecret, events: webhookEvents },
        telegram: { bot_token: tgBotToken, chat_id: tgChatID, events: tgEvents },
      });
      setWebhookSecret('');
      setTgBotToken('');
      toast.show('Notifications saved', 'ok');
      refetch();
    } catch (e: any) {
      toast.show(`Save failed: ${e?.message}`, 'bad');
    } finally { setPending(false); }
  }

  async function test() {
    try {
      await api<any>('/api/notifications/test', { method: 'POST' });
      toast.show('Test event published — check your sink', 'ok');
    } catch (e: any) {
      toast.show(`Test failed: ${e?.message}`, 'bad');
    }
  }

  return (
    <div className="panel panel-pad lg:col-span-2">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-sm font-semibold tracking-tight flex items-center gap-2"><Bell size={14} /> Notifications</h2>
        <button className="btn text-xs" onClick={test}><Send size={12} /> Send test</button>
      </div>
      <div className="grid lg:grid-cols-2 gap-5">
        <div>
          <h3 className="kpi-label mb-2">Webhook</h3>
          <label className="block text-xs mb-1">URL</label>
          <input className="input mb-2" placeholder="https://example.com/webhook" value={webhookUrl} onChange={(e) => setWebhookUrl(e.target.value)} />
          <label className="block text-xs mb-1">Secret <span className="text-muted">(HMAC-SHA256, leave blank to keep)</span></label>
          <input type="password" className="input mb-2" placeholder={data?.notifications.webhook.secret_set ? '••••••• (set)' : ''} value={webhookSecret} onChange={(e) => setWebhookSecret(e.target.value)} />
          <label className="block text-xs mb-1">Events</label>
          <div className="flex flex-wrap gap-1.5">
            {EVENT_KINDS.map((k) => (
              <button key={k} type="button" className={`pill text-xs ${webhookEvents.includes(k) ? 'pill-ok' : 'text-muted'}`} onClick={() => toggle(webhookEvents, setWebhookEvents, k)}>
                <span className="dot" />{k}
              </button>
            ))}
          </div>
        </div>
        <div>
          <h3 className="kpi-label mb-2">Telegram</h3>
          <label className="block text-xs mb-1">Bot token <span className="text-muted">(leave blank to keep)</span></label>
          <input type="password" className="input mb-2" placeholder={data?.notifications.telegram.bot_token_set ? '••••••• (set)' : ''} value={tgBotToken} onChange={(e) => setTgBotToken(e.target.value)} />
          <label className="block text-xs mb-1">Chat ID</label>
          <div className="flex gap-2 mb-2">
            <input className="input flex-1" placeholder="-1001234567890" value={tgChatID} onChange={(e) => setTgChatID(e.target.value)} />
            <DiscoverChatsButton botSet={data?.notifications.telegram.bot_token_set ?? false} onPick={(id) => setTgChatID(String(id))} />
          </div>
          {!tgChatID && (data?.notifications.telegram.bot_token_set ?? false) && (
            <div className="text-xs text-warn dark:text-warn-dark mb-2">
              No chat ID set — Telegram messages won't deliver. Click "Find chat" after sending /start to your bot.
            </div>
          )}
          <label className="block text-xs mb-1">Events</label>
          <div className="flex flex-wrap gap-1.5">
            {EVENT_KINDS.map((k) => (
              <button key={k} type="button" className={`pill text-xs ${tgEvents.includes(k) ? 'pill-ok' : 'text-muted'}`} onClick={() => toggle(tgEvents, setTgEvents, k)}>
                <span className="dot" />{k}
              </button>
            ))}
          </div>
        </div>
      </div>
      <div className="flex justify-end mt-4">
        <button className="btn btn-primary" disabled={pending} onClick={save}>Save changes</button>
      </div>
    </div>
  );
}

function DiscoverChatsButton({ botSet, onPick }: { botSet: boolean; onPick: (id: number) => void }) {
  const [open, setOpen] = useState(false);
  const [chats, setChats] = useState<TelegramChat[] | null>(null);
  const [hint, setHint] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const toast = useToast();

  async function fetchChats() {
    setLoading(true);
    setError('');
    try {
      const res = await api<{ ok: boolean; chats: TelegramChat[]; hint: string }>('/api/notifications/telegram/chats');
      setChats(res.chats);
      setHint(res.hint || '');
    } catch (e: any) {
      setError(e?.message ?? 'request failed');
    } finally {
      setLoading(false);
    }
  }

  function openDialog() {
    if (!botSet) {
      toast.show('Save the bot token first, then retry', 'warn');
      return;
    }
    setOpen(true);
    setChats(null);
    setHint('');
    fetchChats();
  }

  return (
    <>
      <button type="button" className="btn text-xs whitespace-nowrap" onClick={openDialog}>
        <MessageCircle size={12} /> Find chat
      </button>
      {open && (
        <div className="fixed inset-0 z-40 bg-black/40 grid place-items-center p-4" onClick={() => setOpen(false)}>
          <div className="panel panel-pad w-full max-w-md" onClick={(e) => e.stopPropagation()}>
            <div className="flex items-center justify-between mb-2">
              <h3 className="text-base font-semibold flex items-center gap-2"><MessageCircle size={14} /> Find Telegram chat</h3>
              <button className="btn text-xs" disabled={loading} onClick={fetchChats}>
                <RefreshCw size={12} className={loading ? 'animate-spin' : ''} /> Refresh
              </button>
            </div>
            <p className="text-xs text-muted dark:text-muted-dark mb-3">
              Open Telegram, search for your bot, send <code>/start</code>, then click Refresh below. The list shows recent chats that messaged the bot.
            </p>
            {error && <div className="text-xs text-bad dark:text-bad-dark mb-2 break-all">{error}</div>}
            {loading && <div className="text-xs text-muted dark:text-muted-dark">Calling Telegram via the local SOCKS…</div>}
            {!loading && chats && chats.length === 0 && (
              <div className="text-xs text-muted dark:text-muted-dark">{hint || 'No chats yet — send a message to the bot first.'}</div>
            )}
            {!loading && chats && chats.length > 0 && (
              <div className="divide-y divide-border dark:divide-border-dark border border-border dark:border-border-dark rounded-lg">
                {chats.map((c) => (
                  <button
                    key={c.chat_id}
                    type="button"
                    className="w-full text-left px-3 py-2 hover:bg-bg dark:hover:bg-bg-dark"
                    onClick={() => { onPick(c.chat_id); setOpen(false); toast.show(`Chat ID set: ${c.chat_id}`, 'ok'); }}
                  >
                    <div className="flex items-center justify-between">
                      <span className="font-medium text-sm">{c.title || '?'}</span>
                      <span className="text-xs text-muted dark:text-muted-dark font-mono">{c.chat_id}</span>
                    </div>
                    <div className="text-xs text-muted dark:text-muted-dark">{c.type}{c.from ? ` · from ${c.from}` : ''}</div>
                  </button>
                ))}
              </div>
            )}
            <div className="flex justify-end mt-3">
              <button className="btn" onClick={() => setOpen(false)}>Close</button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
