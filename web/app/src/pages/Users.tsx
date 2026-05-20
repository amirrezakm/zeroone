import { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import {
  Ban,
  Copy,
  ExternalLink,
  Filter,
  Gauge,
  HardDrive,
  KeyRound,
  Pencil,
  Plus,
  QrCode,
  RefreshCw,
  RotateCcw,
  Search,
  Share2,
  Sliders,
  Trash2,
  Unplug,
  X,
} from "lucide-react";
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import QRCode from "qrcode";
import PageHeader from "../components/PageHeader";
import StatusPill from "../components/StatusPill";
import {
  useApplyBandwidth,
  useApplyQuota,
  useMetrics,
  useOnline,
  useResetUsage,
  useSyncUsage,
  useSummary,
  useUsage,
  useUserBandwidth,
} from "../api/hooks";
import { post, put, del } from "../api/client";
import { copyText } from "../lib/clipboard";
import { useToast } from "../components/Toast";
import { useQueryClient } from "@tanstack/react-query";
import { bytes, formatTime, formatTimeShort, relativeTime } from "../lib/format";
import type { UserItem } from "../api/types";
import type { OnlineUser } from "../api/hooks";

type FilterKind = "all" | "online" | "enabled" | "disabled" | "banned" | "over_quota";

export default function Users() {
  const summary = useSummary();
  const usage = useUsage();
  const online = useOnline(300);
  const qc = useQueryClient();
  const [params, setParams] = useSearchParams();
  const q = params.get("q") ?? "";
  const [filter, setFilter] = useState<FilterKind>("all");
  const [drawer, setDrawer] = useState<UserItem | null>(null);
  const [adding, setAdding] = useState(false);

  // Prefer the cumulative used_bytes from summary (synced into stack state),
  // fall back to live usage totals when available.
  const usageMap = useMemo(() => {
    const m = new Map<string, number>();
    (summary.data?.user_items ?? []).forEach((u) => {
      if (typeof u.used_bytes === "number") m.set(u.email, u.used_bytes);
    });
    (usage.data?.users ?? []).forEach((u) => {
      if (!m.has(u.email) || (m.get(u.email) ?? 0) < u.total) m.set(u.email, u.total);
    });
    return m;
  }, [usage.data, summary.data]);

  const onlineMap = useMemo(() => {
    const m = new Map<string, OnlineUser>();
    (online.data?.users ?? []).forEach((u) => m.set(u.email, u));
    return m;
  }, [online.data]);

  const users = useMemo(() => {
    let list = summary.data?.user_items ?? [];
    if (q) list = list.filter((u) => u.email.toLowerCase().includes(q.toLowerCase()));
    const now = Date.now() / 1000;
    if (filter === "online") list = list.filter((u) => onlineMap.has(u.email));
    if (filter === "enabled")
      list = list.filter((u) => u.enabled && !(u.banned_until && u.banned_until > now));
    if (filter === "disabled") list = list.filter((u) => !u.enabled);
    if (filter === "banned") list = list.filter((u) => u.banned_until && u.banned_until > now);
    if (filter === "over_quota")
      list = list.filter((u) => u.quota_bytes > 0 && (usageMap.get(u.email) ?? 0) > u.quota_bytes);
    return list;
  }, [summary.data, q, filter, usageMap, onlineMap]);

  function setQ(value: string) {
    const next = new URLSearchParams(params);
    if (value) next.set("q", value);
    else next.delete("q");
    setParams(next, { replace: true });
  }

  return (
    <>
      <PageHeader
        title="Users"
        subtitle={`${summary.data?.users ?? 0} total · ${(summary.data?.user_items ?? []).filter((u) => u.enabled).length} enabled · ${onlineMap.size} online (5m)`}
        actions={
          <div className="flex items-center gap-2">
            <UsageBulkActions allowApply={summary.data?.allow_apply} />
            <button className="btn btn-primary" onClick={() => setAdding(true)}>
              <Plus size={14} /> Add user
            </button>
          </div>
        }
      />

      <section className="mb-5 grid grid-cols-2 gap-3 md:grid-cols-4">
        <Stat
          label="Online users (5m)"
          value={onlineMap.size}
          foot={`${online.data?.unique_client_ips ?? 0} unique IPs`}
          tone="ok"
        />
        <Stat
          label="Devices online (5m)"
          value={(online.data?.users ?? []).reduce((n, u) => n + u.ips.length, 0)}
          foot="distinct client IPs across active users"
        />
        <Stat
          label="Connections (5m)"
          value={online.data?.total_connections ?? 0}
          foot="new flows accepted"
        />
        <Stat
          label="Over quota"
          value={
            (summary.data?.user_items ?? []).filter(
              (u) => u.quota_bytes > 0 && (usageMap.get(u.email) ?? 0) > u.quota_bytes,
            ).length
          }
          tone={
            (summary.data?.user_items ?? []).some(
              (u) => u.quota_bytes > 0 && (usageMap.get(u.email) ?? 0) > u.quota_bytes,
            )
              ? "bad"
              : undefined
          }
        />
      </section>

      <div className="panel mb-4">
        <div className="flex flex-wrap items-center gap-2 border-b border-border px-4 py-3 dark:border-border-dark">
          <div className="flex min-w-[18rem] flex-1 items-center gap-2">
            <Search size={14} className="text-muted" />
            <input
              className="input border-0 bg-transparent px-0 focus:ring-0"
              placeholder="Filter by email…"
              value={q}
              onChange={(e) => setQ(e.target.value)}
            />
          </div>
          <div className="flex flex-wrap items-center gap-1.5 text-xs">
            <Filter size={12} className="text-muted" />
            {(["all", "online", "enabled", "disabled", "banned", "over_quota"] as FilterKind[]).map(
              (f) => (
                <button
                  key={f}
                  onClick={() => setFilter(f)}
                  className={`pill ${filter === f ? "pill-ok" : "text-muted"}`}
                >
                  {f.replace("_", " ")}
                </button>
              ),
            )}
          </div>
        </div>
        <div className="overflow-x-auto">
          <div className="min-w-[80rem]">
            <div className="table-head grid grid-cols-[1.6fr,1fr,1.2fr,1.1fr,1.1fr,1fr,auto]">
              <div>Email</div>
              <div>Status</div>
              <div title="Distinct client IPs in last 5m · new flows /5m">Devices · /5m · IPs</div>
              <div>Used / Quota</div>
              <div>Bandwidth</div>
              <div>Last seen</div>
              <div className="pr-2 text-right">Actions</div>
            </div>
            <div className="divide-y divide-border dark:divide-border-dark">
              {users.length === 0 && (
                <div className="px-4 py-6 text-sm text-muted dark:text-muted-dark">
                  No users match this filter.
                </div>
              )}
              {users.map((u) => (
                <UserRow
                  key={u.email}
                  u={u}
                  usageBytes={usageMap.get(u.email) ?? 0}
                  onlineEntry={onlineMap.get(u.email)}
                  onOpen={() => setDrawer(u)}
                />
              ))}
            </div>
          </div>
        </div>
      </div>

      {drawer && (
        <UserDrawer
          user={drawer}
          usageBytes={usageMap.get(drawer.email) ?? 0}
          onlineEntry={onlineMap.get(drawer.email)}
          onClose={() => setDrawer(null)}
          onChanged={() => qc.invalidateQueries()}
        />
      )}
      {adding && <AddUserDialog onClose={() => setAdding(false)} />}
    </>
  );
}

function UsageBulkActions({ allowApply }: { allowApply?: boolean }) {
  const sync = useSyncUsage();
  const reset = useResetUsage();
  const applyQuota = useApplyQuota();
  const applyBw = useApplyBandwidth();
  const toast = useToast();

  function fire(label: string, m: { mutate: any; isPending: boolean }, danger?: boolean) {
    if (danger && !confirm(`${label}? This affects all users.`)) return;
    m.mutate(undefined, {
      onSuccess: () => toast.show(`${label} ok`, "ok"),
      onError: (e: any) => toast.show(`${label} failed: ${e?.message}`, "bad"),
    });
  }

  return (
    <div className="flex items-center gap-1.5">
      <button
        className="btn text-xs"
        title="Pull cumulative totals from Xray now (in addition to the 60s auto-sync)"
        onClick={() => fire("Sync usage", sync)}
        disabled={sync.isPending}
      >
        <RefreshCw size={12} /> Sync
      </button>
      <button
        className="btn text-xs"
        title="Apply quota plan (auto-disables users over their byte limit)"
        onClick={() => fire("Apply quota", applyQuota)}
        disabled={applyQuota.isPending || !allowApply}
      >
        <Sliders size={12} /> Apply quota
      </button>
      <button
        className="btn text-xs"
        title="Apply bandwidth limits via tc"
        onClick={() => fire("Apply bandwidth", applyBw)}
        disabled={applyBw.isPending || !allowApply}
      >
        <Gauge size={12} /> Apply bandwidth
      </button>
      <button
        className="btn btn-danger text-xs"
        title="Zero cumulative usage totals for all users"
        onClick={() => fire("Reset usage", reset, true)}
        disabled={reset.isPending || !allowApply}
      >
        <RotateCcw size={12} /> Reset
      </button>
    </div>
  );
}

function LiveBandwidthChart({ email }: { email: string }) {
  const live = useUserBandwidth();
  const metrics = useMetrics("1h");
  const cur = live.data?.users?.[email];
  const series = (metrics.data?.samples ?? [])
    .map((s) => ({
      t: s.t,
      up: s.v[`user_${email}_uplink_bps`] ?? 0,
      down: s.v[`user_${email}_downlink_bps`] ?? 0,
    }))
    .filter((p) => p.up > 0 || p.down > 0);
  const fmtBps = (bps: number) =>
    bps > 1_000_000
      ? `${(bps / 1_000_000).toFixed(1)} Mbps`
      : bps > 1000
        ? `${(bps / 1000).toFixed(1)} Kbps`
        : `${bps.toFixed(0)} bps`;
  return (
    <div className="rounded-lg border border-border dark:border-border-dark">
      <div className="flex items-center justify-between border-b border-border px-4 py-2 text-xs font-semibold uppercase tracking-wider text-muted dark:border-border-dark">
        <span>Live bandwidth</span>
        {cur && (
          <span className="font-mono normal-case tracking-normal text-muted">
            ↓ {fmtBps(cur.downlink_bps)} · ↑ {fmtBps(cur.uplink_bps)}
          </span>
        )}
      </div>
      <div className="h-40 px-2 py-1">
        {series.length < 2 ? (
          <div className="flex h-full items-center justify-center text-xs text-muted dark:text-muted-dark">
            Collecting samples — bandwidth refreshes every 60s.
          </div>
        ) : (
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={series}>
              <CartesianGrid stroke="rgba(125,125,135,.15)" vertical={false} />
              <XAxis
                dataKey="t"
                tickFormatter={formatTimeShort}
                fontSize={10}
                stroke="rgba(125,125,135,.7)"
              />
              <YAxis
                fontSize={10}
                stroke="rgba(125,125,135,.7)"
                width={48}
                tickFormatter={fmtBps}
              />
              <Tooltip
                labelFormatter={(v) => formatTimeShort(Number(v))}
                formatter={(v: number) => fmtBps(v)}
                contentStyle={{ borderRadius: 8 }}
              />
              <Line
                dataKey="down"
                name="↓"
                type="monotone"
                stroke="#1a7fbf"
                strokeWidth={1.5}
                dot={false}
                isAnimationActive={false}
              />
              <Line
                dataKey="up"
                name="↑"
                type="monotone"
                stroke="#f38020"
                strokeWidth={1.5}
                dot={false}
                isAnimationActive={false}
              />
            </LineChart>
          </ResponsiveContainer>
        )}
      </div>
    </div>
  );
}

function Stat({
  label,
  value,
  foot,
  tone,
}: {
  label: string;
  value: any;
  foot?: string;
  tone?: "ok" | "bad";
}) {
  const toneCls =
    tone === "ok"
      ? "text-ok dark:text-ok-dark"
      : tone === "bad"
        ? "text-bad dark:text-bad-dark"
        : "";
  return (
    <div className="panel panel-pad">
      <div className="kpi-label">{label}</div>
      <div className={`kpi-value ${toneCls}`}>{value}</div>
      {foot && <div className="kpi-foot mt-1">{foot}</div>}
    </div>
  );
}

function UserRow({
  u,
  usageBytes,
  onlineEntry,
  onOpen,
}: {
  u: UserItem;
  usageBytes: number;
  onlineEntry?: OnlineUser;
  onOpen: () => void;
}) {
  const overQuota = u.quota_bytes > 0 && usageBytes > u.quota_bytes;
  const banned = u.banned_until && u.banned_until > Date.now() / 1000;
  const isOnline = !!onlineEntry;

  return (
    <div
      className="grid cursor-pointer grid-cols-[1.6fr,1fr,1.2fr,1.1fr,1.1fr,1fr,auto] items-center gap-3 px-4 py-3 text-sm hover:bg-bg dark:hover:bg-bg-dark"
      onClick={onOpen}
    >
      <div>
        <div className="flex items-center gap-2">
          <span
            className={`h-2 w-2 rounded-full ${isOnline ? "animate-pulse bg-ok dark:bg-ok-dark" : "bg-muted/40"}`}
          />
          <span className="truncate font-medium">{u.email}</span>
        </div>
        <div className="font-mono text-xs text-muted dark:text-muted-dark">
          {u.uuid.slice(0, 8)}…
        </div>
      </div>
      <div>
        {banned ? (
          <StatusPill ok={false} label={`banned ${relativeTime(u.banned_until)}`} />
        ) : u.enabled ? (
          <StatusPill ok={true} label="enabled" />
        ) : (
          <StatusPill ok={false} label="disabled" />
        )}
      </div>
      <div className="text-xs">
        {onlineEntry ? (
          (() => {
            const devices = onlineEntry.ips.length;
            const cap = u.max_sessions ?? 0;
            const overCap = cap > 0 && devices > cap;
            const deviceClass = overCap
              ? "font-semibold tabular-nums text-bad dark:text-bad-dark"
              : "font-semibold tabular-nums";
            return (
              <>
                <div>
                  <span
                    className={deviceClass}
                    title="Distinct client IPs seen for this user in the last 5 minutes"
                  >
                    {devices}
                  </span>
                  <span className="text-muted dark:text-muted-dark">
                    {" "}
                    {devices === 1 ? "device" : "devices"}
                    {cap > 0 ? ` of ${cap}` : ""} ·{" "}
                  </span>
                  <span className="tabular-nums" title="New flows accepted in last 5 min">
                    {onlineEntry.connections}
                  </span>
                  <span className="text-muted dark:text-muted-dark">/5m</span>
                </div>
                <div
                  className="truncate font-mono text-muted dark:text-muted-dark"
                  title={onlineEntry.ips.join(", ")}
                >
                  {onlineEntry.ips.join(", ") || "—"}
                </div>
                <div className="text-muted dark:text-muted-dark">
                  last {relativeTime(onlineEntry.last_seen)} ·{" "}
                  {onlineEntry.connections_per_min.toFixed(1)}/min
                </div>
              </>
            );
          })()
        ) : (
          <span className="text-muted dark:text-muted-dark">offline</span>
        )}
      </div>
      <div className={overQuota ? "text-bad dark:text-bad-dark" : ""}>
        <div className="font-medium">{bytes(usageBytes)}</div>
        {u.quota_bytes > 0 ? (
          <>
            <div className="text-xs text-muted dark:text-muted-dark">of {bytes(u.quota_bytes)}</div>
            <div className="mt-1 h-1 overflow-hidden rounded-full bg-bg dark:bg-bg-dark">
              <div
                className="h-full"
                style={{
                  width: `${Math.min(100, (usageBytes / u.quota_bytes) * 100).toFixed(1)}%`,
                  background: overQuota ? "#dc2626" : "#1a7fbf",
                }}
              />
            </div>
          </>
        ) : (
          <div className="text-xs text-muted dark:text-muted-dark">unlimited</div>
        )}
      </div>
      <div>
        {u.download_mbps > 0 || u.upload_mbps > 0 ? (
          <span className="font-mono text-xs">
            ↓ {u.download_mbps} ↑ {u.upload_mbps} <span className="text-muted">Mbps</span>
          </span>
        ) : (
          <span className="text-muted">unlimited</span>
        )}
      </div>
      <div className="text-xs">
        {(() => {
          const ts = onlineEntry?.last_seen ?? u.last_seen ?? 0;
          const ip = onlineEntry?.ips[0] ?? u.last_ip ?? "";
          if (!ts) return <span className="text-muted dark:text-muted-dark">never</span>;
          return (
            <>
              <div title={formatTime(ts)}>{relativeTime(ts)}</div>
              {ip && (
                <div className="truncate font-mono text-muted dark:text-muted-dark" title={ip}>
                  {ip}
                </div>
              )}
            </>
          );
        })()}
      </div>
      <div className="flex justify-end gap-1.5" onClick={(e) => e.stopPropagation()}>
        <button className="btn px-2 py-1" onClick={onOpen} title="Open">
          <QrCode size={12} />
        </button>
      </div>
    </div>
  );
}

function UserDrawer({
  user,
  usageBytes,
  onlineEntry,
  onClose,
  onChanged,
}: {
  user: UserItem;
  usageBytes: number;
  onlineEntry?: OnlineUser;
  onClose: () => void;
  onChanged: () => void;
}) {
  const toast = useToast();
  const [tab, setTab] = useState<"connect" | "activity" | "limits" | "danger">("connect");

  return (
    <div className="fixed inset-0 z-40 bg-black/40" onClick={onClose}>
      <aside
        className="absolute bottom-0 right-0 top-0 w-full overflow-y-auto border-l border-border bg-panel dark:border-border-dark dark:bg-panel-dark sm:max-w-lg"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="sticky top-0 z-10 flex items-center justify-between border-b border-border bg-panel px-5 py-4 dark:border-border-dark dark:bg-panel-dark">
          <div>
            <h2 className="font-semibold tracking-tight">{user.email}</h2>
            <div className="font-mono text-xs text-muted dark:text-muted-dark">{user.uuid}</div>
          </div>
          <button onClick={onClose} className="btn px-2">
            <X size={14} />
          </button>
        </div>
        <div className="flex gap-1 overflow-x-auto border-b border-border px-3 py-2 dark:border-border-dark">
          {(
            [
              ["connect", "Connect"],
              ["activity", "Activity"],
              ["limits", "Limits"],
              ["danger", "Danger"],
            ] as const
          ).map(([key, label]) => (
            <button
              key={key}
              onClick={() => setTab(key)}
              className={`rounded-md px-3 py-1.5 text-sm font-medium ${tab === key ? "bg-bg dark:bg-bg-dark" : "text-muted dark:text-muted-dark"}`}
            >
              {label}
            </button>
          ))}
        </div>
        <div className="p-5">
          {tab === "connect" && <ConnectTab user={user} />}
          {tab === "activity" && (
            <ActivityTab user={user} usageBytes={usageBytes} onlineEntry={onlineEntry} />
          )}
          {tab === "limits" && (
            <LimitsTab
              user={user}
              onChanged={onChanged}
              onClose={() => toast.show("Saved", "ok")}
            />
          )}
          {tab === "danger" && <DangerTab user={user} onChanged={onChanged} onClose={onClose} />}
        </div>
      </aside>
    </div>
  );
}

function ConnectTab({ user }: { user: UserItem }) {
  const toast = useToast();
  const [qrs, setQrs] = useState<Record<string, string>>({});
  // Render QR for each link plus one for the primary portal URL (first in
  // the portal_urls list — usually the most reachable CDN-fronted one).
  const primaryPortal =
    user.portal_urls?.find((p) => p.host.startsWith("https://")) ?? user.portal_urls?.[0];
  useEffect(() => {
    user.links.forEach(async (l) => {
      try {
        const dataUrl = await QRCode.toDataURL(l.url, { width: 220, margin: 1 });
        setQrs((q) => ({ ...q, [l.name]: dataUrl }));
      } catch {
        /* skip */
      }
    });
    if (primaryPortal) {
      QRCode.toDataURL(primaryPortal.portal, { width: 220, margin: 1 })
        .then((dataUrl) => setQrs((q) => ({ ...q, __portal: dataUrl })))
        .catch(() => {
          /* skip */
        });
    }
  }, [user, primaryPortal]);
  return (
    <div className="space-y-4">
      {user.portal_urls && user.portal_urls.length > 0 && (
        <div className="rounded-lg border border-accent/40 bg-accent/5 p-3 dark:bg-accent/10">
          <div className="mb-2 flex items-center justify-between">
            <span className="flex items-center gap-1.5 text-sm font-semibold">
              <Share2 size={14} /> User portal
            </span>
            {primaryPortal && (
              <a
                href={primaryPortal.portal}
                target="_blank"
                rel="noreferrer"
                className="btn px-2 py-1 text-xs"
              >
                <ExternalLink size={12} /> Open
              </a>
            )}
          </div>
          <p className="mb-3 text-[11px] text-muted dark:text-muted-dark">
            Share one of these URLs with the user. They open a self-service page with their usage,
            subscription link, QR code, and per-client import instructions. No login needed — the
            token in the URL authenticates.
          </p>
          <div className="mb-3 flex items-start gap-3">
            {qrs.__portal && (
              <img src={qrs.__portal} alt="" className="h-28 w-28 shrink-0 rounded bg-white p-1" />
            )}
            <div className="min-w-0 flex-1 space-y-1.5">
              {user.portal_urls.map((p) => (
                <div key={p.host} className="flex items-center gap-2">
                  <code className="block min-w-0 flex-1 break-all font-mono text-[11px] text-muted dark:text-muted-dark">
                    {p.portal}
                  </code>
                  <button
                    className="btn shrink-0 px-2 py-1 text-xs"
                    onClick={async () => {
                      const ok = await copyText(p.portal);
                      toast.show(ok ? "Copied" : "Copy failed", ok ? "ok" : "bad");
                    }}
                  >
                    <Copy size={11} />
                  </button>
                </div>
              ))}
            </div>
          </div>
          <details className="text-xs">
            <summary className="cursor-pointer text-muted dark:text-muted-dark">
              Subscription URLs (for clients that import URLs directly)
            </summary>
            <div className="mt-2 space-y-1.5">
              {user.portal_urls.map((p) => (
                <div key={p.host} className="flex items-center gap-2">
                  <code className="block min-w-0 flex-1 break-all font-mono text-[11px] text-muted dark:text-muted-dark">
                    {p.sub}
                  </code>
                  <button
                    className="btn shrink-0 px-2 py-1 text-xs"
                    onClick={async () => {
                      const ok = await copyText(p.sub);
                      toast.show(ok ? "Copied" : "Copy failed", ok ? "ok" : "bad");
                    }}
                  >
                    <Copy size={11} />
                  </button>
                </div>
              ))}
            </div>
          </details>
        </div>
      )}
      {user.links.map((l) => (
        <div key={l.name} className="rounded-lg border border-border p-3 dark:border-border-dark">
          <div className="mb-2 flex items-center justify-between">
            <span className="text-sm font-semibold">{l.name}</span>
            <button
              className="btn px-2 py-1 text-xs"
              onClick={async () => {
                const ok = await copyText(l.url);
                toast.show(ok ? "Copied" : "Copy failed — select text manually", ok ? "ok" : "bad");
              }}
            >
              <Copy size={12} /> Copy
            </button>
          </div>
          <div className="flex items-start gap-3">
            {qrs[l.name] && (
              <img src={qrs[l.name]} alt="" className="h-28 w-28 shrink-0 rounded bg-white p-1" />
            )}
            <code className="block break-all font-mono text-[11px] text-muted dark:text-muted-dark">
              {l.url}
            </code>
          </div>
        </div>
      ))}
    </div>
  );
}

function ActivityTab({
  user,
  usageBytes,
  onlineEntry,
}: {
  user: UserItem;
  usageBytes: number;
  onlineEntry?: OnlineUser;
}) {
  const toast = useToast();
  const [pending, setPending] = useState(false);
  const devices = onlineEntry?.ips.length ?? 0;
  const cap = user.max_sessions ?? 0;
  const overCap = cap > 0 && devices > cap;
  async function disconnect() {
    if (
      !confirm(
        `Drop active TCP sessions for ${user.email}? Their client will reconnect (and pick up the latest inbound, e.g. the speed-limited one).`,
      )
    )
      return;
    setPending(true);
    try {
      const res = await post<{ ok: boolean; killed: number; ips: string[] }>(
        "/api/users/disconnect",
        { email: user.email },
      );
      toast.show(
        res.killed > 0
          ? `Disconnected ${res.killed} session${res.killed === 1 ? "" : "s"}`
          : "No active sessions to drop",
        res.killed > 0 ? "ok" : "bad",
      );
    } catch (e: any) {
      toast.show(`Failed: ${e?.message}`, "bad");
    } finally {
      setPending(false);
    }
  }
  return (
    <div className="space-y-4 text-sm">
      <div className="grid grid-cols-3 gap-3">
        <Stat
          label="Devices online (5m)"
          value={cap > 0 ? `${devices} / ${cap}` : devices}
          foot={cap > 0 ? (overCap ? "over device limit" : "within limit") : "no device cap set"}
          tone={overCap ? "bad" : "ok"}
        />
        <Stat
          label="Connections /5m"
          value={onlineEntry?.connections ?? 0}
          foot={`≈ ${(onlineEntry?.connections_per_min ?? 0).toFixed(1)}/min`}
        />
        <Stat
          label="Used (cumulative)"
          value={bytes(usageBytes)}
          foot={user.quota_bytes ? `of ${bytes(user.quota_bytes)} quota` : "unlimited"}
        />
      </div>
      <LiveBandwidthChart email={user.email} />
      <div className="flex items-center justify-between gap-3 rounded-lg border border-border p-3 dark:border-border-dark">
        <div>
          <div className="font-semibold">Drop active sessions</div>
          <div className="mt-0.5 text-xs text-muted dark:text-muted-dark">
            Kills established TCP sockets to this user's recent client IPs across all xray inbounds.
            Useful after changing a speed limit so the client reconnects through the new inbound.
          </div>
        </div>
        <button
          className="btn btn-danger shrink-0"
          onClick={disconnect}
          disabled={pending || devices === 0}
        >
          <Unplug size={14} /> Disconnect
        </button>
      </div>
      <div className="rounded-lg border border-border dark:border-border-dark">
        <div className="border-b border-border px-4 py-2 text-xs font-semibold uppercase tracking-wider text-muted dark:border-border-dark">
          Client IPs
        </div>
        <div className="space-y-1 px-4 py-3">
          {(onlineEntry?.ips ?? []).length === 0 && (
            <div className="text-xs text-muted dark:text-muted-dark">
              No client IPs in the last 5 minutes.
            </div>
          )}
          {(onlineEntry?.ips ?? []).map((ip) => (
            <div key={ip} className="font-mono text-xs">
              {ip}
            </div>
          ))}
        </div>
      </div>
      <div className="rounded-lg border border-border dark:border-border-dark">
        <div className="border-b border-border px-4 py-2 text-xs font-semibold uppercase tracking-wider text-muted dark:border-border-dark">
          Recent destinations
        </div>
        <div className="space-y-1 px-4 py-3">
          {(onlineEntry?.recent_destinations ?? []).length === 0 && (
            <div className="text-xs text-muted dark:text-muted-dark">No recent traffic.</div>
          )}
          {(onlineEntry?.recent_destinations ?? []).map((d) => (
            <div key={d} className="break-all font-mono text-xs">
              {d}
            </div>
          ))}
        </div>
      </div>
      {onlineEntry?.last_seen ? (
        <div className="text-xs text-muted dark:text-muted-dark">
          Last seen {formatTime(onlineEntry.last_seen)} ({relativeTime(onlineEntry.last_seen)})
        </div>
      ) : null}
    </div>
  );
}

function LimitsTab({
  user,
  onChanged,
  onClose,
}: {
  user: UserItem;
  onChanged: () => void;
  onClose: () => void;
}) {
  const toast = useToast();
  const [quotaGB, setQuotaGB] = useState(
    user.quota_bytes ? (user.quota_bytes / 1_000_000_000).toString() : "",
  );
  const [dailyGB, setDailyGB] = useState(
    user.daily_quota_bytes ? (user.daily_quota_bytes / 1_000_000_000).toString() : "",
  );
  const [weeklyGB, setWeeklyGB] = useState(
    user.weekly_quota_bytes ? (user.weekly_quota_bytes / 1_000_000_000).toString() : "",
  );
  const [monthlyGB, setMonthlyGB] = useState(
    user.monthly_quota_bytes ? (user.monthly_quota_bytes / 1_000_000_000).toString() : "",
  );
  const [dailyResetHHMM, setDailyResetHHMM] = useState(user.daily_reset_hhmm || "00:00");
  const [maxSessions, setMaxSessions] = useState(
    user.max_sessions ? user.max_sessions.toString() : "",
  );
  const [downloadMbps, setDownloadMbps] = useState(user.download_mbps.toString());
  const [uploadMbps, setUploadMbps] = useState(user.upload_mbps.toString());
  const [pending, setPending] = useState(false);

  function toBytes(gb: string) {
    const v = parseFloat(gb || "0");
    return isNaN(v) || v <= 0 ? 0 : Math.round(v * 1_000_000_000);
  }

  async function saveQuota() {
    const bytes = toBytes(quotaGB);
    setPending(true);
    try {
      await post("/api/users/quota", { email: user.email, quota_bytes: bytes });
      toast.show(
        bytes === 0 ? "Total quota cleared (unlimited)" : `Total quota set to ${quotaGB} GB`,
        "ok",
      );
      onChanged();
    } catch (e: any) {
      toast.show(`Failed: ${e?.message}`, "bad");
    } finally {
      setPending(false);
    }
  }

  async function savePeriods() {
    const daily = toBytes(dailyGB);
    const weekly = toBytes(weeklyGB);
    const monthly = toBytes(monthlyGB);
    // The default placeholder "00:00" is fine to send as-is; the server
    // accepts empty string as "use default", but sending the explicit
    // value avoids any ambiguity for the operator reading stack.json.
    setPending(true);
    try {
      await post("/api/users/periods", {
        email: user.email,
        daily_quota_bytes: daily,
        weekly_quota_bytes: weekly,
        monthly_quota_bytes: monthly,
        daily_reset_hhmm: dailyResetHHMM,
      });
      toast.show("Period limits saved", "ok");
      onChanged();
    } catch (e: any) {
      toast.show(`Failed: ${e?.message}`, "bad");
    } finally {
      setPending(false);
    }
  }

  async function saveMaxSessions() {
    const n = parseInt(maxSessions || "0", 10);
    setPending(true);
    try {
      await post("/api/users/max-sessions", { email: user.email, max_sessions: isNaN(n) ? 0 : n });
      toast.show(
        !n
          ? "Session limit cleared (unlimited)"
          : `Max ${n} concurrent device${n === 1 ? "" : "s"}`,
        "ok",
      );
      onChanged();
    } catch (e: any) {
      toast.show(`Failed: ${e?.message}`, "bad");
    } finally {
      setPending(false);
    }
  }

  async function saveBandwidth() {
    const dl = parseInt(downloadMbps || "0", 10);
    const ul = parseInt(uploadMbps || "0", 10);
    setPending(true);
    try {
      await post("/api/users/bandwidth", { email: user.email, download_mbps: dl, upload_mbps: ul });
      toast.show(
        dl === 0 && ul === 0 ? "Bandwidth limits cleared" : `Bandwidth set ↓${dl} ↑${ul} Mbps`,
        "ok",
      );
      onChanged();
      onClose();
    } catch (e: any) {
      toast.show(`Failed: ${e?.message}`, "bad");
    } finally {
      setPending(false);
    }
  }

  const usedDaily = user.used_daily_bytes ?? 0;
  const usedWeekly = user.used_weekly_bytes ?? 0;
  const usedMonthly = user.used_monthly_bytes ?? 0;

  return (
    <div className="space-y-5 text-sm">
      <section>
        <h3 className="mb-2 flex items-center gap-2 font-semibold">
          <HardDrive size={14} /> Total quota (lifetime)
        </h3>
        <p className="mb-2 text-xs text-muted dark:text-muted-dark">
          Total bytes the user may consume before being auto-disabled. Leave empty for unlimited.
        </p>
        <div className="flex items-center gap-2">
          <input
            className="input flex-1"
            type="number"
            min="0"
            step="0.1"
            placeholder="0"
            value={quotaGB}
            onChange={(e) => setQuotaGB(e.target.value)}
          />
          <span className="text-sm text-muted">GB</span>
          <button className="btn btn-primary" onClick={saveQuota} disabled={pending}>
            Save
          </button>
        </div>
      </section>

      <section className="border-t border-border pt-4 dark:border-border-dark">
        <h3 className="mb-2 flex items-center gap-2 font-semibold">
          <HardDrive size={14} /> Periodic limits
        </h3>
        <p className="mb-3 text-xs text-muted dark:text-muted-dark">
          Sub-caps under the total quota. Each rolls over independently on its own schedule. Empty /
          0 = no per-period cap.
        </p>
        <div className="grid grid-cols-1 gap-3">
          <PeriodInput
            label="Daily"
            valueGB={dailyGB}
            onValueGB={setDailyGB}
            usedBytes={usedDaily}
            resetAt={user.daily_reset_at}
            extra={
              <label className="block sm:w-32">
                <div className="kpi-label mb-1">Reset at</div>
                <input
                  className="input"
                  type="time"
                  value={dailyResetHHMM}
                  onChange={(e) => setDailyResetHHMM(e.target.value)}
                />
              </label>
            }
          />
          <PeriodInput
            label="Weekly"
            valueGB={weeklyGB}
            onValueGB={setWeeklyGB}
            usedBytes={usedWeekly}
            resetAt={user.weekly_reset_at}
            note="rolls over Monday 00:00 server time"
          />
          <PeriodInput
            label="Monthly"
            valueGB={monthlyGB}
            onValueGB={setMonthlyGB}
            usedBytes={usedMonthly}
            resetAt={user.monthly_reset_at}
            note="rolls over on the 1st 00:00 server time"
          />
        </div>
        <div className="mt-3 flex justify-end">
          <button className="btn btn-primary" onClick={savePeriods} disabled={pending}>
            Save period limits
          </button>
        </div>
      </section>

      <section className="border-t border-border pt-4 dark:border-border-dark">
        <h3 className="mb-2 flex items-center gap-2 font-semibold">
          <Unplug size={14} /> Concurrent device limit
        </h3>
        <p className="mb-2 text-xs text-muted dark:text-muted-dark">
          Maximum number of distinct client IPs allowed to be connected at the same time. Over-limit
          IPs are kicked oldest-first every minute. 0 = unlimited.
        </p>
        <div className="flex items-center gap-2">
          <input
            className="input flex-1"
            type="number"
            min="0"
            placeholder="0"
            value={maxSessions}
            onChange={(e) => setMaxSessions(e.target.value)}
          />
          <span className="text-sm text-muted">devices</span>
          <button className="btn btn-primary" onClick={saveMaxSessions} disabled={pending}>
            Save
          </button>
        </div>
      </section>

      <section className="border-t border-border pt-4 dark:border-border-dark">
        <h3 className="mb-2 flex items-center gap-2 font-semibold">
          <Gauge size={14} /> Speed limit (Bandwidth)
        </h3>
        <p className="mb-2 text-xs text-muted dark:text-muted-dark">
          Per-user download/upload caps via tc. 0 = unlimited.
        </p>
        <div className="mb-2 grid grid-cols-2 gap-2">
          <label className="block">
            <div className="kpi-label mb-1">Download</div>
            <div className="flex items-center gap-2">
              <input
                className="input"
                type="number"
                min="0"
                placeholder="0"
                value={downloadMbps}
                onChange={(e) => setDownloadMbps(e.target.value)}
              />
              <span className="text-xs text-muted">Mbps</span>
            </div>
          </label>
          <label className="block">
            <div className="kpi-label mb-1">Upload</div>
            <div className="flex items-center gap-2">
              <input
                className="input"
                type="number"
                min="0"
                placeholder="0"
                value={uploadMbps}
                onChange={(e) => setUploadMbps(e.target.value)}
              />
              <span className="text-xs text-muted">Mbps</span>
            </div>
          </label>
        </div>
        <button className="btn btn-primary" onClick={saveBandwidth} disabled={pending}>
          Save
        </button>
        {user.bandwidth_port > 0 && (
          <div className="mt-2 text-xs text-muted dark:text-muted-dark">
            Bandwidth-limited inbound on port {user.bandwidth_port}
          </div>
        )}
      </section>

      <RegenerateUUID user={user} onChanged={onChanged} />
    </div>
  );
}

function PeriodInput({
  label,
  valueGB,
  onValueGB,
  usedBytes,
  resetAt,
  extra,
  note,
}: {
  label: string;
  valueGB: string;
  onValueGB: (v: string) => void;
  usedBytes: number;
  resetAt?: number;
  extra?: React.ReactNode;
  note?: string;
}) {
  return (
    <div className="rounded-lg border border-border p-3 dark:border-border-dark">
      <div className="flex flex-wrap items-end gap-3">
        <label className="block min-w-[12rem] flex-1">
          <div className="kpi-label mb-1">{label} cap</div>
          <div className="flex items-center gap-2">
            <input
              className="input flex-1"
              type="number"
              min="0"
              step="0.1"
              placeholder="0"
              value={valueGB}
              onChange={(e) => onValueGB(e.target.value)}
            />
            <span className="text-sm text-muted">GB</span>
          </div>
        </label>
        {extra}
      </div>
      <div className="mt-2 text-xs text-muted dark:text-muted-dark">
        Used so far this period: <span className="font-mono">{bytes(usedBytes)}</span>
        {resetAt ? <> · next reset {relativeTime(resetAt)}</> : null}
        {note ? <> · {note}</> : null}
      </div>
    </div>
  );
}

function RegenerateUUID({ user, onChanged }: { user: UserItem; onChanged: () => void }) {
  const toast = useToast();
  const [pending, setPending] = useState(false);
  async function go() {
    if (
      !confirm(
        `Regenerate UUID for ${user.email}? Existing clients will need a fresh connect link.`,
      )
    )
      return;
    setPending(true);
    try {
      const newUUID = crypto.randomUUID();
      await put("/api/users", {
        old_email: user.email,
        email: user.email,
        uuid: newUUID,
        enabled: user.enabled,
      });
      toast.show("UUID rotated — share new connect link", "ok");
      onChanged();
    } catch (e: any) {
      toast.show(`Failed: ${e?.message}`, "bad");
    } finally {
      setPending(false);
    }
  }
  return (
    <section>
      <h3 className="mb-2 flex items-center gap-2 font-semibold">
        <RefreshCw size={14} /> Rotate UUID
      </h3>
      <p className="mb-2 text-xs text-muted dark:text-muted-dark">
        Generates a new VLESS UUID. Old clients will fail until they update.
      </p>
      <button className="btn" onClick={go} disabled={pending}>
        Regenerate
      </button>
    </section>
  );
}

function DangerTab({
  user,
  onChanged,
  onClose,
}: {
  user: UserItem;
  onChanged: () => void;
  onClose: () => void;
}) {
  const toast = useToast();
  const [banMin, setBanMin] = useState("60");
  const banned = user.banned_until && user.banned_until > Date.now() / 1000;

  async function toggleEnabled() {
    try {
      await put("/api/users", {
        old_email: user.email,
        email: user.email,
        uuid: user.uuid,
        enabled: !user.enabled,
      });
      toast.show(`User ${user.enabled ? "disabled" : "enabled"}`, "ok");
      onChanged();
    } catch (e: any) {
      toast.show(`Failed: ${e?.message}`, "bad");
    }
  }

  async function ban() {
    const minutes = parseInt(banMin || "60", 10);
    try {
      await post("/api/users/ban", { email: user.email, minutes });
      toast.show(`Banned for ${minutes} minutes`, "warn");
      onChanged();
    } catch (e: any) {
      toast.show(`Failed: ${e?.message}`, "bad");
    }
  }

  async function unban() {
    try {
      await post("/api/users/unban", { email: user.email });
      toast.show("Unbanned", "ok");
      onChanged();
    } catch (e: any) {
      toast.show(`Failed: ${e?.message}`, "bad");
    }
  }

  async function remove() {
    if (!confirm(`Delete user ${user.email}? This cannot be undone.`)) return;
    try {
      await del(`/api/users?email=${encodeURIComponent(user.email)}`);
      toast.show("User deleted", "ok");
      onChanged();
      onClose();
    } catch (e: any) {
      toast.show(`Failed: ${e?.message}`, "bad");
    }
  }

  return (
    <div className="space-y-4 text-sm">
      <section>
        <h3 className="mb-2 flex items-center gap-2 font-semibold">
          <Pencil size={14} /> Enable / disable
        </h3>
        <p className="mb-2 text-xs text-muted dark:text-muted-dark">
          Disabled users cannot connect. State persists across restarts.
        </p>
        <button className="btn" onClick={toggleEnabled}>
          {user.enabled ? "Disable" : "Enable"}
        </button>
      </section>

      <section>
        <h3 className="mb-2 flex items-center gap-2 font-semibold">
          <Ban size={14} /> Temporary ban
        </h3>
        <p className="mb-2 text-xs text-muted dark:text-muted-dark">
          Time-limited block — auto-lifts after duration.{" "}
          {banned ? `Currently banned ${relativeTime(user.banned_until!)}.` : ""}
        </p>
        <div className="flex items-center gap-2">
          <input
            className="input flex-1"
            type="number"
            min="1"
            value={banMin}
            onChange={(e) => setBanMin(e.target.value)}
          />
          <span className="text-xs text-muted">minutes</span>
          <button className="btn btn-danger" onClick={ban}>
            Ban
          </button>
          {banned && (
            <button className="btn" onClick={unban}>
              Unban now
            </button>
          )}
        </div>
      </section>

      <section className="border-t border-border pt-4 dark:border-border-dark">
        <h3 className="mb-2 flex items-center gap-2 font-semibold text-bad dark:text-bad-dark">
          <Trash2 size={14} /> Delete user
        </h3>
        <p className="mb-2 text-xs text-muted dark:text-muted-dark">
          Permanent. The connect links become invalid immediately.
        </p>
        <button className="btn btn-danger" onClick={remove}>
          Delete {user.email}
        </button>
      </section>
    </div>
  );
}

function AddUserDialog({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [email, setEmail] = useState("");
  const [pending, setPending] = useState(false);
  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setPending(true);
    try {
      await post("/api/users", { email });
      toast.show("User added", "ok");
      qc.invalidateQueries({ queryKey: ["summary"] });
      onClose();
    } catch (err: any) {
      toast.show(`Add failed: ${err?.message}`, "bad");
    } finally {
      setPending(false);
    }
  }
  return (
    <div className="fixed inset-0 z-40 grid place-items-center bg-black/40 p-4" onClick={onClose}>
      <form
        onSubmit={submit}
        className="panel panel-pad w-full max-w-md"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="mb-3 flex items-center gap-2 text-base font-semibold">
          <KeyRound size={16} /> New user
        </h2>
        <label className="mb-1 block text-xs text-muted dark:text-muted-dark">Email</label>
        <input
          className="input mb-3"
          placeholder="user@panel"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          autoFocus
          required
        />
        <div className="flex justify-end gap-2">
          <button type="button" className="btn" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className="btn btn-primary" disabled={pending || !email}>
            Create
          </button>
        </div>
      </form>
    </div>
  );
}
