import { useEffect, useState } from 'react';
import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';
import { Activity, AlertTriangle, ArrowRight, Check, History, Settings2, X } from 'lucide-react';
import PageHeader from '../components/PageHeader';
import StatusPill from '../components/StatusPill';
import { useFailover, useFailoverHistory, useHealth, useMetrics, useSetFailoverMode, useSystem, useTestConnect } from '../api/hooks';
import { useToast } from '../components/Toast';
import { bytes, formatTime, formatTimeShort, relativeTime } from '../lib/format';
import type { FailoverHistoryEntry } from '../api/hooks';
import type { FailoverModeName } from '../api/types';

export default function Tunnels() {
  const health = useHealth();
  const failover = useFailover();
  const metrics = useMetrics('1h');
  const system = useSystem();

  const tunnels = health.data?.tunnels ?? [];
  const samples = metrics.data?.samples ?? [];
  const decision = failover.data?.decision;
  const ifaceMap = new Map((system.data?.tunnels ?? []).map((t) => [t.name, t]));

  return (
    <>
      <PageHeader
        title="Tunnels"
        subtitle={decision ? `Effective outbound: ${decision.effective.outbound_tag}${decision.effective.interface ? ` via ${decision.effective.interface}` : ''}` : ''}
      />

      <FailoverModeCard />

      <FailoverHistory />

      <div className="grid lg:grid-cols-2 gap-5">
        {tunnels.map((t) => {
          const series = samples.map((s) => ({ t: s.t, v: s.v[`tunnel_${t.name}_latency_ms`] ?? 0 }));
          const ifc = ifaceMap.get(t.interface);
          const txDropPct = ifc && ifc.tx_bytes > 0 && ifc.tx_dropped != null
            ? (ifc.tx_dropped / Math.max(1, ifc.tx_bytes / 1500)) * 100
            : 0;
          return (
            <div key={t.name} className="panel">
              <div className="px-5 py-4 flex items-center justify-between border-b border-border dark:border-border-dark">
                <div>
                  <h3 className="text-sm font-semibold tracking-tight">{t.name}</h3>
                  <p className="text-xs text-muted dark:text-muted-dark">{t.systemd_unit} · {t.interface} · priority {t.priority}</p>
                </div>
                <StatusPill ok={t.healthy} label={t.healthy ? 'Healthy' : t.up ? 'Degraded' : 'Down'} />
              </div>
              <div className="px-5 py-4 grid grid-cols-3 gap-3 text-sm">
                <div><div className="kpi-label">IPv4</div><div className="font-mono">{t.ipv4 || '—'}</div></div>
                <div><div className="kpi-label">Probe</div><div className="font-mono text-xs">{t.probe || '—'}</div></div>
                <div><div className="kpi-label">Latency</div><div className="font-mono">{t.latency_ms != null ? `${t.latency_ms} ms` : '—'}</div></div>
              </div>
              {ifc && (
                <div className="px-5 pb-4 grid grid-cols-2 gap-3 text-xs">
                  <div>
                    <div className="kpi-label">RX / TX</div>
                    <div className="font-mono">{bytes(ifc.rx_bytes)} ↓ · {bytes(ifc.tx_bytes)} ↑</div>
                  </div>
                  <div>
                    <div className="kpi-label flex items-center gap-1">
                      Dropped
                      {(ifc.tx_dropped ?? 0) > 1000 && <AlertTriangle size={10} className="text-warn dark:text-warn-dark" />}
                    </div>
                    <div className={`font-mono ${(ifc.tx_dropped ?? 0) > 1000 ? 'text-warn dark:text-warn-dark' : ''}`}>
                      {ifc.rx_dropped ?? 0} ↓ · {ifc.tx_dropped ?? 0} ↑
                      {txDropPct > 0.1 && <span className="text-muted ml-1">({txDropPct.toFixed(2)}%)</span>}
                    </div>
                  </div>
                </div>
              )}
              <div className="h-32 px-2 pb-2">
                <ResponsiveContainer width="100%" height="100%">
                  <LineChart data={series}>
                    <CartesianGrid stroke="rgba(125,125,135,.15)" vertical={false} />
                    <XAxis dataKey="t" tickFormatter={formatTimeShort} fontSize={10} stroke="rgba(125,125,135,.7)" />
                    <YAxis fontSize={10} stroke="rgba(125,125,135,.7)" width={32} />
                    <Tooltip labelFormatter={(v) => formatTimeShort(Number(v))} formatter={(v: number) => `${v} ms`} contentStyle={{ borderRadius: 8 }} />
                    <Line dataKey="v" type="monotone" stroke="#f38020" strokeWidth={1.5} dot={false} isAnimationActive={false} />
                  </LineChart>
                </ResponsiveContainer>
              </div>
              <ConnectivityTester tunnelName={t.name} interfaceName={t.interface} />
              {t.error && <div className="px-5 py-3 text-xs text-bad dark:text-bad-dark border-t border-border dark:border-border-dark">{t.error}</div>}
            </div>
          );
        })}
      </div>
    </>
  );
}

const MODE_LABELS: Record<FailoverModeName, string> = {
  auto: 'Auto',
  manual: 'Manual',
  preferred: 'Preferred',
};
const MODE_DESCRIPTIONS: Record<FailoverModeName, string> = {
  auto: 'Pick the first healthy tunnel by priority. Fully automatic.',
  manual: 'Pin to the chosen tunnel. If it dies, fall over — but don’t auto-return when it recovers.',
  preferred: 'Bias toward the chosen tunnel: fall over on failure, return automatically once it’s healthy again.',
};

function FailoverModeCard() {
  const failover = useFailover();
  const health = useHealth();
  const setMode = useSetFailoverMode();
  const toast = useToast();

  const serverMode = (failover.data?.mode ?? 'auto') as FailoverModeName;
  const serverPreferred = failover.data?.preferred_tunnel ?? '';
  const tunnels = health.data?.tunnels ?? [];

  const [mode, setLocalMode] = useState<FailoverModeName>(serverMode);
  const [preferred, setLocalPreferred] = useState<string>(serverPreferred);

  // Sync local state when the server-side mode changes — but not while a
  // mutation is in flight (the polling refetch would otherwise reset the
  // user's selection mid-apply and feel broken).
  useEffect(() => {
    if (!setMode.isPending) setLocalMode(serverMode);
  }, [serverMode, setMode.isPending]);
  useEffect(() => {
    if (!setMode.isPending) setLocalPreferred(serverPreferred);
  }, [serverPreferred, setMode.isPending]);

  // When the user flips to manual/preferred without a stored tunnel, default
  // to the currently effective interface so they don't have to pick blind.
  useEffect(() => {
    if ((mode === 'manual' || mode === 'preferred') && !preferred && tunnels.length > 0) {
      const active = failover.data?.decision?.effective?.interface;
      const match = tunnels.find((t) => t.interface === active) ?? tunnels[0];
      if (match) setLocalPreferred(match.name);
    }
  }, [mode, preferred, tunnels, failover.data?.decision?.effective?.interface]);

  const dirty = mode !== serverMode || (mode !== 'auto' && preferred !== serverPreferred);
  const needsPreferred = mode !== 'auto' && !preferred;

  // Show extended progress text if the apply takes more than ~2s
  // (typically because xray.service is restarting).
  const [slowApply, setSlowApply] = useState(false);
  useEffect(() => {
    if (!setMode.isPending) {
      setSlowApply(false);
      return;
    }
    const t = setTimeout(() => setSlowApply(true), 2000);
    return () => clearTimeout(t);
  }, [setMode.isPending]);

  function apply() {
    setMode.mutate(
      { mode, preferred_tunnel: mode === 'auto' ? undefined : preferred },
      {
        onSuccess: (res) => {
          const iface = res.effective?.interface ? ` (${res.effective.interface})` : '';
          toast.show(`Mode set to ${MODE_LABELS[res.mode]}${iface}`, 'ok');
        },
        onError: (e: any) => {
          toast.show(`Failed: ${e?.message ?? 'request failed'}`, 'bad');
        },
      },
    );
  }

  const effective = failover.data?.decision?.effective;

  return (
    <section className="panel mb-5">
      <div className="px-5 py-3 border-b border-border dark:border-border-dark flex items-center justify-between">
        <h2 className="text-sm font-semibold tracking-tight flex items-center gap-2">
          <Settings2 size={14} /> Failover mode
        </h2>
        <span className="text-xs text-muted dark:text-muted-dark">
          {effective ? <>active: <span className="font-mono">{effective.outbound_tag}{effective.interface ? ` / ${effective.interface}` : ''}</span></> : '—'}
        </span>
      </div>
      <div className="px-5 py-4 space-y-3">
        <div className="flex flex-wrap gap-2" role="radiogroup" aria-label="Failover mode">
          {(['auto', 'manual', 'preferred'] as FailoverModeName[]).map((m) => (
            <button
              key={m}
              type="button"
              role="radio"
              aria-checked={mode === m}
              className={`btn text-xs ${mode === m ? 'btn-primary' : ''}`}
              onClick={() => setLocalMode(m)}
            >
              {MODE_LABELS[m]}
            </button>
          ))}
        </div>
        <p className="text-xs text-muted dark:text-muted-dark">{MODE_DESCRIPTIONS[mode]}</p>
        {mode !== 'auto' && (
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <label className="kpi-label" htmlFor="preferred-tunnel">Tunnel</label>
            <select
              id="preferred-tunnel"
              className="input text-xs"
              value={preferred}
              onChange={(e) => setLocalPreferred(e.target.value)}
            >
              <option value="">— pick a tunnel —</option>
              {tunnels.map((t) => (
                <option key={t.name} value={t.name}>
                  {t.name} ({t.interface}){!t.healthy ? ' · down' : ''}
                </option>
              ))}
            </select>
          </div>
        )}
        <div className="flex items-center gap-3 pt-1">
          <button
            type="button"
            className="btn btn-primary text-xs"
            disabled={!dirty || needsPreferred || setMode.isPending}
            onClick={apply}
          >
            {setMode.isPending ? (slowApply ? 'Restarting xray…' : 'Applying…') : 'Apply'}
          </button>
          {dirty && !needsPreferred && (
            <span className="text-xs text-muted dark:text-muted-dark">
              unsaved: {MODE_LABELS[mode]}{mode !== 'auto' && preferred ? ` · ${preferred}` : ''}
            </span>
          )}
          {needsPreferred && (
            <span className="text-xs text-warn dark:text-warn-dark">pick a tunnel to enable {MODE_LABELS[mode].toLowerCase()} mode</span>
          )}
        </div>
      </div>
    </section>
  );
}

function FailoverHistory() {
  const { data } = useFailoverHistory();
  const entries = (data?.entries ?? []).slice().reverse();
  return (
    <section className="panel mb-5">
      <div className="px-5 py-3 border-b border-border dark:border-border-dark flex items-center justify-between">
        <h2 className="text-sm font-semibold tracking-tight flex items-center gap-2">
          <History size={14} /> Failover history
        </h2>
        <span className="text-xs text-muted dark:text-muted-dark">
          {entries.length} transition{entries.length === 1 ? '' : 's'} · last {data?.retention_hours ?? 48}h
        </span>
      </div>
      {entries.length === 0 ? (
        <div className="px-5 py-6 text-sm text-muted dark:text-muted-dark">
          No transitions recorded — your tunnel hasn't flipped since the daemon started recording.
        </div>
      ) : (
        <div className="divide-y divide-border dark:divide-border-dark max-h-72 overflow-auto">
          {entries.map((e, i) => <FailoverHistoryRow key={`${e.t}-${i}`} entry={e} />)}
        </div>
      )}
    </section>
  );
}

function FailoverHistoryRow({ entry: e }: { entry: FailoverHistoryEntry }) {
  const fromLabel = `${e.from.outbound_tag}${e.from.interface ? ' / ' + e.from.interface : ''}`;
  const toLabel = `${e.to.outbound_tag}${e.to.interface ? ' / ' + e.to.interface : ''}`;
  const sameInterface = fromLabel === toLabel;
  const failed = !!e.error;
  return (
    <div className="px-5 py-2.5 grid grid-cols-[20px,160px,1fr,1fr] gap-3 text-sm items-center">
      <div title={failed ? 'failed' : 'succeeded'} className={failed ? 'text-bad dark:text-bad-dark' : 'text-ok dark:text-ok-dark'}>
        {failed ? <X size={16} strokeWidth={3} /> : <Check size={16} strokeWidth={3} />}
      </div>
      <div className="text-xs">
        <div className="font-mono">{formatTime(e.t)}</div>
        <div className="text-muted dark:text-muted-dark">{relativeTime(e.t)}</div>
      </div>
      <div className="flex items-center gap-2 font-mono text-xs">
        {sameInterface ? (
          // Mode-only change (e.g. manual↔preferred on the same pinned tunnel) — show one label, no arrow.
          <span className={failed ? 'text-bad dark:text-bad-dark line-through' : ''}>{toLabel}</span>
        ) : (
          <>
            <span>{fromLabel}</span>
            <ArrowRight size={12} className="text-muted shrink-0" />
            <span className={failed ? 'text-bad dark:text-bad-dark' : 'text-ok dark:text-ok-dark'}>{toLabel}</span>
          </>
        )}
      </div>
      <div className="text-xs text-muted dark:text-muted-dark truncate" title={e.error || e.reason}>
        {failed ? <span className="text-bad dark:text-bad-dark">{e.reason} — {e.error}</span> : e.reason}
      </div>
    </div>
  );
}

function ConnectivityTester({ tunnelName, interfaceName }: { tunnelName: string; interfaceName: string }) {
  const test = useTestConnect();
  const toast = useToast();
  const [target, setTarget] = useState('1.1.1.1');
  const [port, setPort] = useState('443');
  const [last, setLast] = useState<string>('');

  function run() {
    test.mutate(
      { route: tunnelName, target, port: parseInt(port, 10) || 443 },
      {
        onSuccess: (res) => {
          const dur = `${res.duration_ms}ms`;
          setLast(`${res.ok ? '✓' : '✗'} ${target}:${port} via ${interfaceName} — ${res.status} (${dur})${res.error ? ` — ${res.error}` : ''}`);
          toast.show(res.ok ? `Connected in ${dur}` : `Failed: ${res.error || res.status}`, res.ok ? 'ok' : 'bad');
        },
        onError: (e: any) => {
          setLast(`✗ ${e?.message ?? 'request failed'}`);
          toast.show(`Test failed: ${e?.message}`, 'bad');
        },
      },
    );
  }

  return (
    <div className="px-5 py-3 border-t border-border dark:border-border-dark">
      <div className="kpi-label mb-2 flex items-center gap-1.5"><Activity size={12} /> TCP probe</div>
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <input className="input flex-1 min-w-[8rem]" placeholder="1.1.1.1" value={target} onChange={(e) => setTarget(e.target.value)} />
        <input className="input w-20" placeholder="443" value={port} onChange={(e) => setPort(e.target.value)} />
        <button className="btn btn-primary text-xs" onClick={run} disabled={test.isPending}>
          {test.isPending ? 'Testing…' : 'Test'}
        </button>
      </div>
      {last && <div className="mt-2 text-xs font-mono text-muted dark:text-muted-dark break-all">{last}</div>}
    </div>
  );
}
