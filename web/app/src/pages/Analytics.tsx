import { useState } from 'react';
import { Area, AreaChart, Bar, BarChart, CartesianGrid, Cell, Legend, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';
import { Globe } from 'lucide-react';
import PageHeader from '../components/PageHeader';
import { useMetrics, useSummary, useTopDestinations, useTraffic } from '../api/hooks';
import { bps, bytes, formatTimeShort } from '../lib/format';
import clsx from 'clsx';

type Range = '1h' | '24h';

export default function Analytics() {
  const [range, setRange] = useState<Range>('1h');
  const metrics = useMetrics(range);
  const summary = useSummary();

  const samples = metrics.data?.samples ?? [];
  const tunnelNames = (summary.data?.tunnels ?? []).map((t) => t.name);
  const colors = ['#f38020', '#1a7fbf', '#22c08a', '#d54065'];

  const bandwidthData = samples.map((s) => {
    const row: Record<string, number> = { t: s.t };
    tunnelNames.forEach((n) => {
      row[`${n}_rx`] = s.v[`tunnel_${n}_rx_bps`] ?? 0;
      row[`${n}_tx`] = s.v[`tunnel_${n}_tx_bps`] ?? 0;
    });
    return row;
  });
  const latencyData = samples.map((s) => {
    const row: Record<string, number> = { t: s.t };
    tunnelNames.forEach((n) => {
      row[`${n}_lat`] = s.v[`tunnel_${n}_latency_ms`] ?? 0;
    });
    return row;
  });
  const sysData = samples.map((s) => ({ t: s.t, cpu: s.v.cpu_pct ?? 0, ram: s.v.ram_pct ?? 0 }));

  return (
    <>
      <PageHeader
        title="Analytics"
        subtitle="Tunnel bandwidth, latency, and host load"
        actions={
          <div className="flex rounded-lg border border-border dark:border-border-dark overflow-hidden text-sm">
            {(['1h', '24h'] as Range[]).map((r) => (
              <button
                key={r}
                onClick={() => setRange(r)}
                className={clsx('px-3 py-1.5 font-semibold', range === r ? 'bg-bg dark:bg-bg-dark' : 'bg-panel dark:bg-panel-dark text-muted dark:text-muted-dark')}
              >
                {r}
              </button>
            ))}
          </div>
        }
      />

      <section className="grid lg:grid-cols-2 gap-5 mb-5">
        <div className="panel panel-pad">
          <h2 className="text-sm font-semibold tracking-tight mb-3">Tunnel bandwidth (in/out)</h2>
          <div className="h-64">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={bandwidthData}>
                <CartesianGrid stroke="rgba(125,125,135,.15)" vertical={false} />
                <XAxis dataKey="t" tickFormatter={formatTimeShort} fontSize={11} stroke="rgba(125,125,135,.7)" />
                <YAxis fontSize={11} stroke="rgba(125,125,135,.7)" tickFormatter={(v) => bps(Number(v))} width={70} />
                <Tooltip labelFormatter={(v) => formatTimeShort(Number(v))} formatter={(v: number) => bps(v)} contentStyle={{ background: 'var(--tw-prose-body)', borderRadius: 8 }} />
                <Legend wrapperStyle={{ fontSize: 11 }} />
                {tunnelNames.map((n, i) => (
                  <Area key={`${n}-rx`} dataKey={`${n}_rx`} stackId={n} stroke={colors[i % colors.length]} fill={colors[i % colors.length]} fillOpacity={0.18} isAnimationActive={false} />
                ))}
                {tunnelNames.map((n, i) => (
                  <Area key={`${n}-tx`} dataKey={`${n}_tx`} stackId={`${n}-out`} stroke={colors[i % colors.length]} fill={colors[i % colors.length]} fillOpacity={0.08} strokeDasharray="3 3" isAnimationActive={false} />
                ))}
              </AreaChart>
            </ResponsiveContainer>
          </div>
        </div>

        <div className="panel panel-pad">
          <h2 className="text-sm font-semibold tracking-tight mb-3">Probe latency (ms)</h2>
          <div className="h-64">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={latencyData}>
                <CartesianGrid stroke="rgba(125,125,135,.15)" vertical={false} />
                <XAxis dataKey="t" tickFormatter={formatTimeShort} fontSize={11} stroke="rgba(125,125,135,.7)" />
                <YAxis fontSize={11} stroke="rgba(125,125,135,.7)" width={40} />
                <Tooltip labelFormatter={(v) => formatTimeShort(Number(v))} contentStyle={{ borderRadius: 8 }} />
                <Legend wrapperStyle={{ fontSize: 11 }} />
                {tunnelNames.map((n, i) => (
                  <Line key={n} dataKey={`${n}_lat`} type="monotone" stroke={colors[i % colors.length]} strokeWidth={1.5} dot={false} isAnimationActive={false} />
                ))}
              </LineChart>
            </ResponsiveContainer>
          </div>
        </div>
      </section>

      <TrafficByAction range={range} samples={samples} />

      <TopDestinations />

      <section className="panel panel-pad">
        <h2 className="text-sm font-semibold tracking-tight mb-3">Host load</h2>
        <div className="h-64">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={sysData}>
              <CartesianGrid stroke="rgba(125,125,135,.15)" vertical={false} />
              <XAxis dataKey="t" tickFormatter={formatTimeShort} fontSize={11} stroke="rgba(125,125,135,.7)" />
              <YAxis fontSize={11} stroke="rgba(125,125,135,.7)" width={40} domain={[0, 100]} />
              <Tooltip labelFormatter={(v) => formatTimeShort(Number(v))} formatter={(v: number) => `${v.toFixed(0)}%`} contentStyle={{ borderRadius: 8 }} />
              <Legend wrapperStyle={{ fontSize: 11 }} />
              <Area dataKey="cpu" name="CPU" stroke="#f38020" fill="#f38020" fillOpacity={0.18} isAnimationActive={false} />
              <Area dataKey="ram" name="RAM" stroke="#1a7fbf" fill="#1a7fbf" fillOpacity={0.18} isAnimationActive={false} />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </section>
    </>
  );
}

const TAG_COLORS: Record<string, string> = {
  proxy: '#1a7fbf',
  'priority-proxy': '#7e22ce',
  direct: '#22c08a',
  block: '#dc2626',
  fallback: '#f38020',
};

function colorFor(tag: string, idx: number): string {
  return TAG_COLORS[tag] ?? ['#f38020', '#1a7fbf', '#22c08a', '#d54065'][idx % 4];
}

function TopDestinations() {
  const { data } = useTopDestinations(20);
  const items = data?.items ?? [];
  const totalShown = items.reduce((s, i) => s + i.requests, 0);
  const totalAll = data?.total ?? 0;
  return (
    <section className="panel mb-5">
      <div className="px-5 py-3 border-b border-border dark:border-border-dark flex items-center justify-between">
        <h2 className="text-sm font-semibold tracking-tight flex items-center gap-2">
          <Globe size={14} /> Top destinations
        </h2>
        <span className="text-xs text-muted dark:text-muted-dark">
          {items.length} of {totalAll.toLocaleString()} requests · last {data?.window ?? '48h'}
        </span>
      </div>
      {items.length === 0 ? (
        <div className="px-5 py-6 text-sm text-muted dark:text-muted-dark">
          Collecting destinations from the Xray journal — give it a minute.
        </div>
      ) : (
        <div>
          <div className="table-head grid grid-cols-[60px,1fr,160px,180px] px-4 py-2">
            <div className="text-right pr-2">#</div>
            <div>Destination</div>
            <div className="text-right">Requests</div>
            <div>Share</div>
          </div>
          <div className="divide-y divide-border dark:divide-border-dark">
            {items.map((it, idx) => {
              const pct = totalAll > 0 ? (it.requests / totalAll) * 100 : 0;
              return (
                <div key={it.destination} className="grid grid-cols-[60px,1fr,160px,180px] px-4 py-2 text-sm items-center gap-2">
                  <div className="text-right pr-2 text-muted dark:text-muted-dark tabular-nums">{idx + 1}</div>
                  <div className="font-mono text-xs truncate" title={it.destination}>{it.destination}</div>
                  <div className="text-right font-medium tabular-nums">{it.requests.toLocaleString()}</div>
                  <div className="flex items-center gap-2">
                    <div className="h-1.5 flex-1 rounded-full bg-bg dark:bg-bg-dark overflow-hidden">
                      <div className="h-full rounded-full" style={{ width: `${pct}%`, background: '#1a7fbf' }} />
                    </div>
                    <span className="text-xs text-muted dark:text-muted-dark tabular-nums w-10 text-right">{pct.toFixed(1)}%</span>
                  </div>
                </div>
              );
            })}
          </div>
          <div className="px-5 py-2 border-t border-border dark:border-border-dark text-xs text-muted dark:text-muted-dark">
            Showing top {items.length} · {((totalShown / Math.max(1, totalAll)) * 100).toFixed(1)}% of all tracked requests · 48 h retention
          </div>
        </div>
      )}
    </section>
  );
}

function TrafficByAction({ range, samples }: { range: '1h' | '24h'; samples: any[] }) {
  const traffic = useTraffic();
  const tags = Object.keys(traffic.data?.outbounds ?? {});

  const totalsRow = tags.map((tag) => {
    const t = traffic.data!.outbounds[tag];
    return { tag, uplink: t.uplink, downlink: t.downlink, total: t.uplink + t.downlink };
  }).sort((a, b) => b.total - a.total);

  const totalAll = totalsRow.reduce((sum, r) => sum + r.total, 0);

  const rateData = samples.map((s) => {
    const row: Record<string, number | string> = { t: s.t };
    for (const tag of tags) {
      row[`${tag}_dl`] = s.v[`outbound_${tag}_downlink_bps`] ?? 0;
    }
    return row;
  });

  return (
    <section className="grid lg:grid-cols-3 gap-5 mb-5">
      <div className="panel panel-pad lg:col-span-2">
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-semibold tracking-tight">Traffic by routing action ({range})</h2>
          <span className="text-xs text-muted dark:text-muted-dark">downlink, bytes/sec</span>
        </div>
        <div className="h-64">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={rateData}>
              <CartesianGrid stroke="rgba(125,125,135,.15)" vertical={false} />
              <XAxis dataKey="t" tickFormatter={formatTimeShort} fontSize={11} stroke="rgba(125,125,135,.7)" />
              <YAxis fontSize={11} stroke="rgba(125,125,135,.7)" tickFormatter={(v) => bps(Number(v))} width={70} />
              <Tooltip labelFormatter={(v) => formatTimeShort(Number(v))} formatter={(v: number) => bps(v)} contentStyle={{ borderRadius: 8 }} />
              <Legend wrapperStyle={{ fontSize: 11 }} />
              {tags.map((tag, i) => (
                <Area key={tag} dataKey={`${tag}_dl`} name={tag} stackId="dl" stroke={colorFor(tag, i)} fill={colorFor(tag, i)} fillOpacity={0.22} isAnimationActive={false} />
              ))}
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </div>

      <div className="panel panel-pad">
        <h2 className="text-sm font-semibold tracking-tight mb-1">Cumulative bytes per tag</h2>
        <p className="text-xs text-muted dark:text-muted-dark mb-3">Since Xray (re)started — total {bytes(totalAll)}</p>
        <div className="space-y-2">
          {totalsRow.length === 0 && <div className="text-sm text-muted dark:text-muted-dark">Waiting for first stats tick…</div>}
          {totalsRow.map((r, i) => {
            const pct = totalAll > 0 ? (r.total / totalAll) * 100 : 0;
            return (
              <div key={r.tag}>
                <div className="flex items-center justify-between text-xs mb-1">
                  <span className="font-mono">{r.tag}</span>
                  <span className="font-mono text-muted dark:text-muted-dark">{bytes(r.total)} <span className="opacity-50">· {pct.toFixed(0)}%</span></span>
                </div>
                <div className="h-1.5 rounded-full bg-bg dark:bg-bg-dark overflow-hidden">
                  <div className="h-full rounded-full" style={{ width: `${pct}%`, background: colorFor(r.tag, i) }} />
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </section>
  );
}
