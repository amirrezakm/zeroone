import { Globe, Plus, Shield, ShieldOff, Wifi } from "lucide-react";
import PageHeader from "../components/PageHeader";
import { useSummary, useTraffic } from "../api/hooks";
import { post, del } from "../api/client";
import { useQueryClient } from "@tanstack/react-query";
import { useToast } from "../components/Toast";
import { useState } from "react";
import { bytes } from "../lib/format";

type RuleAction = "block" | "direct" | "proxy";

type RuleRow = { kind: string; matcher: string; action: RuleAction; source: "auto" | "manual" };

export default function Rules() {
  const summary = useSummary();
  const traffic = useTraffic();
  const qc = useQueryClient();
  const toast = useToast();
  const [adding, setAdding] = useState(false);

  const data = summary.data;
  const rows: RuleRow[] = [];
  data?.block_domains?.forEach((d) =>
    rows.push({ kind: "domain", matcher: d, action: "block", source: "auto" }),
  );
  data?.manual_blocks?.forEach((d) =>
    rows.push({ kind: "domain", matcher: d, action: "block", source: "manual" }),
  );
  data?.direct_domains?.forEach((d) =>
    rows.push({ kind: "domain", matcher: d, action: "direct", source: "manual" }),
  );

  const blockCount = rows.filter((r) => r.action === "block").length;
  const directCount = rows.filter((r) => r.action === "direct").length;
  const proxyCount = rows.filter((r) => r.action === "proxy").length;

  const ob = traffic.data?.outbounds ?? {};
  const bytesFor = (tag: string) => (ob[tag]?.uplink ?? 0) + (ob[tag]?.downlink ?? 0);
  const blockBytes = bytesFor("block");
  const directBytes = bytesFor("direct");
  const proxyBytes = bytesFor("proxy") + bytesFor("priority-proxy") + bytesFor("fallback");

  async function handleDeleteDirect(domain: string) {
    if (!confirm(`Remove direct rule for ${domain}?`)) return;
    try {
      await del(`/api/direct-domains?domain=${encodeURIComponent(domain)}`);
      toast.show("Rule removed", "ok");
      qc.invalidateQueries({ queryKey: ["summary"] });
    } catch (e: any) {
      toast.show(`Failed: ${e?.message}`, "bad");
    }
  }

  return (
    <>
      <PageHeader
        title="Rules"
        subtitle={`Routing decisions evaluated top-to-bottom — ${rows.length} active`}
        actions={
          <button className="btn btn-primary" onClick={() => setAdding(true)}>
            <Plus size={14} /> New direct domain
          </button>
        }
      />

      <section className="mb-5 grid grid-cols-3 gap-3">
        <Stat
          label="Block"
          count={blockCount}
          foot={`${bytes(blockBytes)} blocked since restart`}
          icon={<ShieldOff size={14} />}
          tone="bad"
        />
        <Stat
          label="Direct (bypass)"
          count={directCount}
          foot={`${bytes(directBytes)} sent direct`}
          icon={<Wifi size={14} />}
          tone="ok"
        />
        <Stat
          label="Proxy"
          count={proxyCount || "—"}
          foot={`${bytes(proxyBytes)} via tunnel`}
          icon={<Shield size={14} />}
          tone="default"
        />
      </section>

      <div className="panel">
        <div className="table-head grid grid-cols-[1fr_3fr_1fr_1fr_auto] px-4 py-2">
          <div>Kind</div>
          <div>Matcher</div>
          <div>Action</div>
          <div>Source</div>
          <div></div>
        </div>
        <div className="divide-border dark:divide-border-dark divide-y">
          {rows.map((r, i) => (
            <div
              key={i}
              className="grid grid-cols-[1fr_3fr_1fr_1fr_auto] items-center gap-3 px-4 py-2.5 text-sm"
            >
              <div className="flex items-center gap-2">
                <Globe size={12} className="text-muted" />
                {r.kind}
              </div>
              <code className="font-mono text-xs break-all">{r.matcher}</code>
              <div>
                <span
                  className={`pill ${r.action === "block" ? "pill-bad" : r.action === "direct" ? "pill-ok" : "text-muted"}`}
                >
                  <span className="dot" />
                  {r.action}
                </span>
              </div>
              <div className="text-muted dark:text-muted-dark text-xs">{r.source}</div>
              <div className="flex justify-end">
                {r.source === "manual" && r.action === "direct" && (
                  <button
                    className="btn btn-danger px-2 py-1 text-xs"
                    onClick={() => handleDeleteDirect(r.matcher)}
                  >
                    Remove
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      </div>

      {adding && <AddDirectDialog onClose={() => setAdding(false)} />}
    </>
  );
}

function Stat({
  label,
  count,
  icon,
  tone,
  foot,
}: {
  label: string;
  count: any;
  icon: React.ReactNode;
  tone: "ok" | "bad" | "default";
  foot?: string;
}) {
  const toneCls =
    tone === "ok"
      ? "text-ok dark:text-ok-dark"
      : tone === "bad"
        ? "text-bad dark:text-bad-dark"
        : "";
  return (
    <div className="panel panel-pad">
      <div className="kpi-label flex items-center gap-2">
        {icon} {label}
      </div>
      <div className={`kpi-value ${toneCls}`}>{count}</div>
      {foot && <div className="kpi-foot mt-1">{foot}</div>}
    </div>
  );
}

type DomainScope = "subdomains" | "exact" | "raw";

function AddDirectDialog({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [domain, setDomain] = useState("");
  const [scope, setScope] = useState<DomainScope>("subdomains");
  const [pending, setPending] = useState(false);

  // Mirror the backend's NormalizeDomainRule for live preview. The server
  // re-normalizes too — this is best-effort so the user can see what they're
  // about to save. URLs/ports/paths get stripped; case is folded.
  function normalize(input: string): string {
    let s = input.trim();
    const knownPrefix = ["domain:", "full:", "regexp:", "geosite:", "ext:"].find((p) =>
      s.startsWith(p),
    );
    if (knownPrefix) return s;
    s = s.replace(/^[a-z]+:\/\//i, "");
    s = s.split(/[/?#]/, 1)[0];
    s = s.replace(/:\d+$/, "");
    return s.toLowerCase();
  }

  const cleaned = normalize(domain);
  const hasKnownPrefix = ["domain:", "full:", "regexp:", "geosite:", "ext:"].some((p) =>
    cleaned.startsWith(p),
  );
  const finalRule = hasKnownPrefix
    ? cleaned
    : scope === "exact"
      ? `full:${cleaned}`
      : scope === "subdomains"
        ? `domain:${cleaned}`
        : cleaned;

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!cleaned) {
      toast.show("Enter a domain", "bad");
      return;
    }
    setPending(true);
    try {
      const res = await post<{ ok: boolean; applied?: boolean; apply_error?: string }>(
        "/api/direct-domains",
        { domain: finalRule },
      );
      if (res.applied) {
        toast.show(`Added — live now: ${finalRule}`, "ok");
      } else if (res.apply_error) {
        toast.show(`Saved but apply failed: ${res.apply_error}`, "bad");
      } else {
        toast.show(`Saved: ${finalRule} — click Apply to make live`, "warn");
      }
      qc.invalidateQueries({ queryKey: ["summary"] });
      qc.invalidateQueries({ queryKey: ["apply-plan"] });
      onClose();
    } catch (e: any) {
      toast.show(`Failed: ${e?.message}`, "bad");
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
        <h2 className="mb-1 text-base font-semibold">Add direct domain</h2>
        <p className="text-muted dark:text-muted-dark mb-3 text-xs">
          Traffic to this rule will go out via the local interface (eth0) instead of the proxy.
        </p>

        <label className="kpi-label mb-1">Domain</label>
        <input
          className="input mb-2"
          placeholder="example.com  or  https://example.com/"
          value={domain}
          onChange={(e) => setDomain(e.target.value)}
          autoFocus
          required
        />

        {!hasKnownPrefix && (
          <>
            <label className="kpi-label mb-1">Match</label>
            <div className="mb-3 grid gap-2">
              <ScopeChoice
                active={scope === "subdomains"}
                onClick={() => setScope("subdomains")}
                title="All subdomains (incl. itself)"
                detail={
                  cleaned
                    ? `matches ${cleaned}, www.${cleaned}, api.${cleaned} …`
                    : "recommended for most cases"
                }
                example={cleaned ? `domain:${cleaned}` : "domain:example.com"}
              />
              <ScopeChoice
                active={scope === "exact"}
                onClick={() => setScope("exact")}
                title="Exact match only"
                detail={
                  cleaned ? `matches only ${cleaned} (not subdomains)` : "only the exact host"
                }
                example={cleaned ? `full:${cleaned}` : "full:example.com"}
              />
            </div>
          </>
        )}

        {cleaned && (
          <div className="text-muted dark:text-muted-dark mb-3 text-xs">
            Will save as{" "}
            <code className="text-foreground dark:text-foreground-dark font-mono">{finalRule}</code>
          </div>
        )}

        <div className="flex justify-end gap-2">
          <button type="button" className="btn" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className="btn btn-primary" disabled={pending || !cleaned}>
            {pending ? "Saving…" : "Add"}
          </button>
        </div>
      </form>
    </div>
  );
}

function ScopeChoice({
  active,
  onClick,
  title,
  detail,
  example,
}: {
  active: boolean;
  onClick: () => void;
  title: string;
  detail: string;
  example: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`rounded-lg border px-3 py-2 text-left transition ${
        active
          ? "border-ok bg-ok/5 dark:border-ok-dark dark:bg-ok-dark/5"
          : "border-border hover:bg-bg dark:border-border-dark dark:hover:bg-bg-dark"
      }`}
    >
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium">{title}</span>
        <span
          className={`h-2 w-2 rounded-full ${active ? "bg-ok dark:bg-ok-dark" : "bg-muted/40"}`}
        />
      </div>
      <div className="text-muted dark:text-muted-dark mt-0.5 text-xs">{detail}</div>
      <div className="text-muted/80 dark:text-muted-dark/80 mt-1 font-mono text-[11px]">
        {example}
      </div>
    </button>
  );
}
