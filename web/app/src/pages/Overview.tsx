import {
  Activity,
  ArrowDown,
  ArrowUp,
  Cpu,
  MemoryStick,
  ShieldCheck,
  Users as UsersIcon,
} from "lucide-react";
import KPICard from "../components/KPICard";
import StatusPill from "../components/StatusPill";
import PageHeader from "../components/PageHeader";
import {
  useFailover,
  useHealth,
  useMetrics,
  useOnline,
  useSummary,
  useUsage,
  useAudit,
} from "../api/hooks";
import { bps, bytes, formatTime, relativeTime } from "../lib/format";

export default function Overview() {
  const summary = useSummary();
  const health = useHealth();
  const failover = useFailover();
  const metrics = useMetrics("1h");
  const usage = useUsage();
  const audit = useAudit(15);

  const online = useOnline(300);
  const samples = metrics.data?.samples ?? [];
  const latest = samples[samples.length - 1];
  const cpuSeries = samples.map((s) => ({ t: s.t, v: s.v.cpu_pct ?? 0 }));
  const ramSeries = samples.map((s) => ({ t: s.t, v: s.v.ram_pct ?? 0 }));

  // Sum tunnel rates
  function rateSeries(direction: "rx" | "tx") {
    return samples.map((s) => {
      let total = 0;
      Object.entries(s.v).forEach(([k, v]) => {
        if (k.startsWith("tunnel_") && k.endsWith(`_${direction}_bps`)) total += v;
      });
      return { t: s.t, v: total };
    });
  }
  const rxSeries = rateSeries("rx");
  const txSeries = rateSeries("tx");
  const latestRx = rxSeries[rxSeries.length - 1]?.v ?? 0;
  const latestTx = txSeries[txSeries.length - 1]?.v ?? 0;

  const decision = failover.data?.decision;
  const failoverOK =
    decision?.effective.outbound_tag === decision?.desired.outbound_tag && !decision?.pending;

  const topUsers = (usage.data?.users ?? [])
    .slice()
    .sort((a, b) => b.total - a.total)
    .slice(0, 5);
  const tunnels = health.data?.tunnels ?? [];

  return (
    <>
      <PageHeader
        title="Overview"
        subtitle={summary.data?.public_ip ? `Server ${summary.data.public_ip} · live` : "live"}
      />

      <section className="mb-6 grid grid-cols-2 gap-3 md:grid-cols-3 lg:grid-cols-6">
        <KPICard
          label="CPU"
          value={`${(latest?.v.cpu_pct ?? 0).toFixed(0)}%`}
          hint={summary.data ? "load average" : "—"}
          series={cpuSeries}
          icon={<Cpu size={14} />}
          tone={latest?.v.cpu_pct && latest.v.cpu_pct > 80 ? "bad" : "default"}
        />
        <KPICard
          label="RAM"
          value={`${(latest?.v.ram_pct ?? 0).toFixed(0)}%`}
          hint={latest ? bytes(latest.v.ram_used ?? 0) : "—"}
          series={ramSeries}
          icon={<MemoryStick size={14} />}
          tone={latest?.v.ram_pct && latest.v.ram_pct > 85 ? "warn" : "default"}
        />
        <KPICard
          label="Inbound"
          value={bps(latestRx)}
          hint="now"
          series={rxSeries}
          icon={<ArrowDown size={14} />}
          tone="ok"
        />
        <KPICard
          label="Outbound"
          value={bps(latestTx)}
          hint="now"
          series={txSeries}
          icon={<ArrowUp size={14} />}
          tone="ok"
        />
        <KPICard
          label="Online users"
          value={online.data?.users.length ?? 0}
          hint={`${online.data?.active_tcp_sessions ?? 0} active sessions · ${online.data?.unique_client_ips ?? 0} IPs`}
          icon={<UsersIcon size={14} />}
          tone="ok"
        />
        <KPICard
          label="Failover"
          value={decision?.effective.outbound_tag ?? "—"}
          hint={decision?.reason ?? ""}
          icon={<ShieldCheck size={14} />}
          tone={failoverOK ? "ok" : "warn"}
        />
      </section>

      <section className="panel mb-6">
        <div className="border-border dark:border-border-dark flex items-center justify-between border-b px-5 py-3">
          <h2 className="text-sm font-semibold tracking-tight">Online users (last 5 min)</h2>
          <span className="text-muted dark:text-muted-dark text-xs">live from xray logs</span>
        </div>
        <div className="divide-border dark:divide-border-dark divide-y">
          {(online.data?.users ?? []).length === 0 && (
            <div className="text-muted dark:text-muted-dark px-5 py-6 text-sm">
              No users active in the last 5 minutes.
            </div>
          )}
          {(online.data?.users ?? []).map((u) => (
            <div
              key={u.email}
              className="grid grid-cols-[1.4fr_1fr_1fr_2fr] items-center gap-3 px-5 py-3 text-sm"
            >
              <div className="flex min-w-0 items-center gap-2">
                <span className="bg-ok dark:bg-ok-dark h-2 w-2 shrink-0 animate-pulse rounded-full" />
                <span className="truncate font-medium">{u.email}</span>
              </div>
              <div className="text-xs">
                <span className="font-semibold">{u.active_sessions}</span>
                <span className="text-muted dark:text-muted-dark">
                  {" "}
                  active · {u.connections_per_min.toFixed(1)}/min
                </span>
              </div>
              <div className="truncate font-mono text-xs">{u.ips.join(", ")}</div>
              <div className="text-muted dark:text-muted-dark truncate text-xs">
                last {relativeTime(u.last_seen)} · {(u.recent_destinations[0] ?? "—").slice(0, 56)}
              </div>
            </div>
          ))}
        </div>
      </section>

      <section className="mb-6 grid gap-5 lg:grid-cols-3">
        <div className="panel panel-pad lg:col-span-2">
          <div className="mb-3 flex items-center justify-between">
            <h2 className="text-sm font-semibold tracking-tight">Tunnels</h2>
            <span className="text-muted dark:text-muted-dark text-xs">
              {health.data
                ? `Updated ${relativeTime(Math.floor(new Date(health.data.generated_at).getTime() / 1000))}`
                : "—"}
            </span>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            {tunnels.map((t) => (
              <div
                key={t.name}
                className="border-border dark:border-border-dark rounded-lg border p-3"
              >
                <div className="mb-2 flex items-center justify-between">
                  <div>
                    <div className="text-sm font-semibold">{t.name}</div>
                    <div className="text-muted dark:text-muted-dark text-xs">
                      {t.interface} · {t.type}
                    </div>
                  </div>
                  <StatusPill
                    ok={t.healthy}
                    label={t.healthy ? "Healthy" : t.up ? "Degraded" : "Down"}
                  />
                </div>
                <div className="grid grid-cols-2 gap-2 text-xs">
                  <div>
                    <div className="text-muted">IPv4</div>
                    <div className="font-mono">{t.ipv4 || "—"}</div>
                  </div>
                  <div>
                    <div className="text-muted">Latency</div>
                    <div className="font-mono">
                      {t.latency_ms != null ? `${t.latency_ms} ms` : "—"}
                    </div>
                  </div>
                  <div className="col-span-2">
                    <div className="text-muted">Probe</div>
                    <div className="font-mono">{t.probe || "—"}</div>
                  </div>
                </div>
                {t.error && (
                  <div className="text-bad dark:text-bad-dark mt-2 text-xs">{t.error}</div>
                )}
              </div>
            ))}
          </div>
        </div>

        <div className="panel panel-pad">
          <h2 className="mb-3 text-sm font-semibold tracking-tight">Top users (by traffic)</h2>
          <div className="space-y-2">
            {topUsers.length === 0 && (
              <div className="text-muted dark:text-muted-dark text-sm">
                No usage data yet — sync to populate.
              </div>
            )}
            {topUsers.map((u) => (
              <div key={u.email} className="flex items-center justify-between text-sm">
                <span className="truncate font-medium">{u.email}</span>
                <span className="text-muted dark:text-muted-dark font-mono">{bytes(u.total)}</span>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="panel">
        <div className="border-border dark:border-border-dark flex items-center justify-between border-b px-5 py-4">
          <h2 className="flex items-center gap-2 text-sm font-semibold tracking-tight">
            <Activity size={14} /> Recent activity
          </h2>
          <span className="text-muted dark:text-muted-dark text-xs">audit log</span>
        </div>
        <div className="divide-border dark:divide-border-dark divide-y">
          {(audit.data?.entries ?? []).length === 0 && (
            <div className="text-muted dark:text-muted-dark px-5 py-6 text-sm">
              No actions recorded yet.
            </div>
          )}
          {(audit.data?.entries ?? []).map((e, i) => (
            <div key={i} className="flex items-center justify-between px-5 py-2.5 text-sm">
              <div className="flex min-w-0 items-center gap-3">
                <span className="pill text-muted dark:text-muted-dark">
                  <span className="dot bg-muted" />
                  {e.action}
                </span>
                <span className="text-muted dark:text-muted-dark truncate">{e.target || "—"}</span>
              </div>
              <div className="text-muted dark:text-muted-dark text-xs whitespace-nowrap">
                {e.actor} · {formatTime(e.t)}
              </div>
            </div>
          ))}
        </div>
      </section>
    </>
  );
}
