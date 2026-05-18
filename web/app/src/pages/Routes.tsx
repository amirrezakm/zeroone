import { useState } from 'react';
import { ArrowDown, ArrowUp, Copy, Network, Pencil, Plus, Trash2 } from 'lucide-react';
import PageHeader from '../components/PageHeader';
import { useSummary, useTraffic } from '../api/hooks';
import { post, put, del } from '../api/client';
import { useQueryClient } from '@tanstack/react-query';
import { useToast } from '../components/Toast';
import { copyText } from '../lib/clipboard';
import { bytes } from '../lib/format';
import type { SocksItem } from '../api/types';

export default function RoutesPage() {
  const summary = useSummary();
  const socks = summary.data?.socks_items ?? [];
  const [editing, setEditing] = useState<SocksItem | null>(null);
  const [adding, setAdding] = useState(false);
  const qc = useQueryClient();
  const toast = useToast();

  async function remove(name: string, username: string) {
    if (!confirm(`Delete SOCKS inbound "${name}"? Existing clients will lose access.`)) return;
    try {
      await del(`/api/socks?username=${encodeURIComponent(username)}`);
      toast.show('SOCKS inbound removed', 'ok');
      qc.invalidateQueries({ queryKey: ['summary'] });
    } catch (e: any) {
      toast.show(`Delete failed: ${e?.message}`, 'bad');
    }
  }

  return (
    <>
      <PageHeader
        title="Routes"
        subtitle="Public SOCKS inbounds and direct-bypass list"
        actions={
          <button className="btn btn-primary" onClick={() => setAdding(true)}>
            <Plus size={14} /> New SOCKS
          </button>
        }
      />

      <InboundTrafficPanel />

      <section className="panel mb-5">
        <div className="px-5 py-3 border-b border-border dark:border-border-dark flex items-center justify-between">
          <h2 className="text-sm font-semibold tracking-tight">SOCKS inbounds</h2>
          <span className="text-xs text-muted dark:text-muted-dark">{socks.length} configured</span>
        </div>
        <div className="divide-y divide-border dark:divide-border-dark">
          {socks.map((s) => (
            <div key={s.name} className="px-5 py-3 grid sm:grid-cols-[2fr,1fr,1fr,1fr,auto] gap-2 text-sm items-center">
              <div className="font-medium truncate">{s.name}</div>
              <div className="font-mono text-xs">{s.listen}:{s.port}</div>
              <div className="text-muted dark:text-muted-dark text-xs">{s.username}</div>
              <div className="flex items-center gap-1">
                <button
                  className="btn text-xs"
                  title="Copy SOCKS URL"
                  onClick={async () => {
                    const url = s.links?.[0]?.url ?? `socks5://${s.username}:${s.password}@${summary.data?.public_ip ?? s.listen}:${s.port}`;
                    const ok = await copyText(url);
                    toast.show(ok ? 'Copied' : 'Copy failed — select text manually', ok ? 'ok' : 'bad');
                  }}
                ><Copy size={12} /></button>
                <span className="text-xs text-muted dark:text-muted-dark">{s.links.length} links</span>
              </div>
              <div className="flex items-center gap-1 justify-end">
                <button className="btn px-2 py-1" onClick={() => setEditing(s)} title="Edit"><Pencil size={12} /></button>
                <button className="btn btn-danger px-2 py-1" onClick={() => remove(s.name, s.username)} title="Delete"><Trash2 size={12} /></button>
              </div>
            </div>
          ))}
          {socks.length === 0 && <div className="px-5 py-6 text-sm text-muted dark:text-muted-dark">No SOCKS inbounds. Click "New SOCKS" to create one.</div>}
        </div>
      </section>

      <section className="grid lg:grid-cols-2 gap-5">
        <div className="panel">
          <div className="px-5 py-3 border-b border-border dark:border-border-dark"><h2 className="text-sm font-semibold tracking-tight">Direct domains</h2></div>
          <div className="px-5 py-3 max-h-72 overflow-auto text-xs font-mono space-y-1">
            {(summary.data?.direct_domains ?? []).map((d) => <div key={d}>{d}</div>)}
            {(summary.data?.direct_domains ?? []).length === 0 && <div className="text-muted dark:text-muted-dark text-sm font-sans">No direct rules.</div>}
          </div>
        </div>
        <div className="panel">
          <div className="px-5 py-3 border-b border-border dark:border-border-dark"><h2 className="text-sm font-semibold tracking-tight">Block rules</h2></div>
          <div className="px-5 py-3 max-h-72 overflow-auto text-xs font-mono space-y-1">
            {[...(summary.data?.block_domains ?? []), ...(summary.data?.manual_blocks ?? [])].map((d) => <div key={d}>{d}</div>)}
          </div>
        </div>
      </section>

      {(adding || editing) && (
        <SOCKSDialog
          initial={editing ?? undefined}
          onClose={() => { setAdding(false); setEditing(null); }}
          onSaved={() => { setAdding(false); setEditing(null); qc.invalidateQueries({ queryKey: ['summary'] }); }}
        />
      )}
    </>
  );
}

function InboundTrafficPanel() {
  const { data } = useTraffic();
  const inbounds = data?.inbounds ?? {};
  const rates = data?.inbound_rates ?? {};
  const rows = Object.entries(inbounds)
    .map(([tag, totals]) => ({
      tag,
      uplink: totals.uplink,
      downlink: totals.downlink,
      total: totals.uplink + totals.downlink,
      uplinkBps: rates[tag]?.uplink_bps ?? 0,
      downlinkBps: rates[tag]?.downlink_bps ?? 0,
    }))
    .sort((a, b) => b.total - a.total);

  function fmtBps(bps: number) {
    if (!bps) return '—';
    if (bps > 1_000_000) return `${(bps / 1_000_000).toFixed(1)} Mbps`;
    if (bps > 1000) return `${(bps / 1000).toFixed(1)} Kbps`;
    return `${bps.toFixed(0)} bps`;
  }

  return (
    <section className="panel mb-5">
      <div className="px-5 py-3 border-b border-border dark:border-border-dark flex items-center justify-between">
        <h2 className="text-sm font-semibold tracking-tight flex items-center gap-2">
          <Network size={14} /> Inbound traffic split
        </h2>
        <span className="text-xs text-muted dark:text-muted-dark">
          {rows.length} active · totals since Xray start
        </span>
      </div>
      {rows.length === 0 ? (
        <div className="px-5 py-6 text-sm text-muted dark:text-muted-dark">No inbound traffic recorded yet.</div>
      ) : (
        <>
          <div className="table-head grid grid-cols-[1.4fr,1fr,1fr,1fr,1fr] px-4 py-2">
            <div>Tag</div>
            <div>Total</div>
            <div title="Cumulative bytes from clients to server">↑ Uplink</div>
            <div title="Cumulative bytes from server to clients">↓ Downlink</div>
            <div title="Current rate (last 60s sample)">Rate now</div>
          </div>
          <div className="divide-y divide-border dark:divide-border-dark">
            {rows.map((r) => (
              <div key={r.tag} className="grid grid-cols-[1.4fr,1fr,1fr,1fr,1fr] px-4 py-2.5 text-sm items-center gap-2">
                <div className="font-mono text-xs truncate" title={r.tag}>{r.tag}</div>
                <div className="font-medium">{bytes(r.total)}</div>
                <div className="text-xs"><ArrowUp size={10} className="inline mr-1 text-muted" />{bytes(r.uplink)}</div>
                <div className="text-xs"><ArrowDown size={10} className="inline mr-1 text-muted" />{bytes(r.downlink)}</div>
                <div className="text-xs font-mono text-muted dark:text-muted-dark">
                  {r.downlinkBps + r.uplinkBps > 0
                    ? <>↓ {fmtBps(r.downlinkBps)} · ↑ {fmtBps(r.uplinkBps)}</>
                    : 'idle'}
                </div>
              </div>
            ))}
          </div>
        </>
      )}
    </section>
  );
}

function SOCKSDialog({ initial, onClose, onSaved }: { initial?: SocksItem; onClose: () => void; onSaved: () => void }) {
  const editing = !!initial;
  const toast = useToast();
  const [name, setName] = useState(initial?.name ?? '');
  const [listen, setListen] = useState(initial?.listen ?? '0.0.0.0');
  const [port, setPort] = useState(String(initial?.port ?? 1080));
  const [username, setUsername] = useState(initial?.username ?? '');
  const [password, setPassword] = useState(initial?.password ?? '');
  const [pending, setPending] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setPending(true);
    const body = { name, listen, port: parseInt(port, 10) || 1080, username, password };
    try {
      if (editing) {
        await put('/api/socks', { ...body, old_username: initial!.username });
      } else {
        await post('/api/socks', body);
      }
      toast.show(editing ? 'SOCKS updated' : 'SOCKS created', 'ok');
      onSaved();
    } catch (err: any) {
      toast.show(`Save failed: ${err?.message}`, 'bad');
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="fixed inset-0 z-40 bg-black/40 grid place-items-center p-4" onClick={onClose}>
      <form onSubmit={submit} className="panel panel-pad w-full max-w-md space-y-3" onClick={(e) => e.stopPropagation()}>
        <h2 className="text-base font-semibold">{editing ? 'Edit SOCKS inbound' : 'New SOCKS inbound'}</h2>
        <div>
          <label className="kpi-label">Name</label>
          <input className="input" placeholder="router" value={name} onChange={(e) => setName(e.target.value)} required autoFocus disabled={editing} />
        </div>
        <div className="grid grid-cols-[2fr,1fr] gap-2">
          <div>
            <label className="kpi-label">Listen</label>
            <input className="input" placeholder="0.0.0.0" value={listen} onChange={(e) => setListen(e.target.value)} required />
          </div>
          <div>
            <label className="kpi-label">Port</label>
            <input className="input" type="number" min="1" max="65535" value={port} onChange={(e) => setPort(e.target.value)} required />
          </div>
        </div>
        <div className="grid grid-cols-2 gap-2">
          <div>
            <label className="kpi-label">Username</label>
            <input className="input" value={username} onChange={(e) => setUsername(e.target.value)} required />
          </div>
          <div>
            <label className="kpi-label">Password</label>
            <input className="input font-mono" value={password} onChange={(e) => setPassword(e.target.value)} required />
          </div>
        </div>
        <div className="flex justify-end gap-2 pt-1">
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={pending || !name || !username || !password}>
            {pending ? 'Saving…' : editing ? 'Save' : 'Create'}
          </button>
        </div>
      </form>
    </div>
  );
}
