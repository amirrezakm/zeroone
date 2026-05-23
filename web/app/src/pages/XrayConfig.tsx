import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { Zap, RefreshCw } from "lucide-react";
import PageHeader from "../components/PageHeader";
import SnapshotTitleDialog from "../components/SnapshotTitleDialog";
import { useToast } from "../components/Toast";
import { useApplyPlan, useApplyXray, useGeneratedXray, useSnapshots } from "../api/hooks";
import { formatTime, relativeTime } from "../lib/format";

// XrayConfig is the single hub for inspecting and re-applying the
// rendered xray config. xray.json is owned by the daemon, so this page
// is intentionally read-only — the Apply button is the supported way to
// push pending stack changes to the live process, and it always opens
// the snapshot title dialog so the operator records *why* they applied.
export default function XrayConfig() {
  const generated = useGeneratedXray();
  const plan = useApplyPlan();
  const snapshots = useSnapshots();
  const apply = useApplyXray();
  const toast = useToast();
  const [dialogOpen, setDialogOpen] = useState(false);

  const generatedText = useMemo(
    () => (generated.data ? JSON.stringify(generated.data, null, 2) : ""),
    [generated.data],
  );

  const allowApply = plan.data?.allow_apply ?? false;
  const changed = plan.data?.changed ?? false;
  const recent = (snapshots.data?.snapshots ?? []).slice(0, 5);

  return (
    <>
      <PageHeader
        title="Xray Config"
        subtitle="Single place to view the rendered xray.json and apply pending changes. Every apply takes a titled snapshot."
        actions={
          <>
            <button
              className="btn"
              onClick={() => {
                generated.refetch();
                plan.refetch();
              }}
            >
              <RefreshCw size={14} /> Refresh
            </button>
            <button
              className="btn btn-primary"
              disabled={!allowApply || apply.isPending}
              title={
                allowApply
                  ? "Apply pending stack changes to live Xray"
                  : "Apply is disabled — start zeroone with -allow-apply"
              }
              onClick={() => setDialogOpen(true)}
            >
              <Zap size={14} /> Apply
            </button>
          </>
        }
      />

      <div className="grid gap-4 lg:grid-cols-[2fr_1fr]">
        <section className="panel">
          <div className="border-border dark:border-border-dark flex items-center justify-between border-b px-4 py-2">
            <div className="text-sm font-medium">Generated xray.json</div>
            <div className="text-muted dark:text-muted-dark text-xs">
              {plan.isLoading
                ? "checking plan…"
                : changed
                  ? "pending changes vs live"
                  : "in sync with live"}
            </div>
          </div>
          <div className="bg-bg dark:bg-bg-dark max-h-[60vh] overflow-auto p-3">
            {generated.isLoading && (
              <div className="text-muted dark:text-muted-dark text-sm">Loading…</div>
            )}
            {generated.isError && (
              <div className="text-bad dark:text-bad-dark text-sm">
                Failed to load generated config.
              </div>
            )}
            {!generated.isLoading && !generated.isError && (
              <pre className="font-mono text-xs leading-snug break-all whitespace-pre-wrap">
                {generatedText}
              </pre>
            )}
          </div>
        </section>

        <section className="space-y-4">
          <div className="panel">
            <div className="border-border dark:border-border-dark border-b px-4 py-2">
              <div className="text-sm font-medium">Apply status</div>
            </div>
            <div className="space-y-2 px-4 py-3 text-xs">
              <Row label="Allowed">{allowApply ? "yes" : "no (start with -allow-apply)"}</Row>
              <Row label="Pending changes">{changed ? "yes" : "no"}</Row>
              <Row label="Live config path">
                <span className="font-mono">{plan.data?.config_path ?? "—"}</span>
              </Row>
              {plan.data?.error && (
                <div className="text-bad dark:text-bad-dark">{plan.data.error}</div>
              )}
            </div>
          </div>

          <div className="panel">
            <div className="border-border dark:border-border-dark flex items-center justify-between border-b px-4 py-2">
              <div className="text-sm font-medium">Recent snapshots</div>
              <Link to="/snapshots" className="text-accent text-xs hover:underline">
                View all →
              </Link>
            </div>
            <div className="divide-border dark:divide-border-dark divide-y">
              {recent.length === 0 && (
                <div className="text-muted dark:text-muted-dark px-4 py-3 text-xs">
                  No snapshots yet.
                </div>
              )}
              {recent.map((s) => (
                <div key={s.id} className="px-4 py-2 text-xs">
                  <div className="flex items-center justify-between gap-2">
                    <span className="truncate font-medium">
                      {s.title || <em className="text-muted dark:text-muted-dark">untitled</em>}
                    </span>
                    <span
                      className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] ${
                        s.source === "auto" ? "bg-warn/10 text-warn-dark" : "bg-ok/10 text-ok-dark"
                      }`}
                    >
                      {s.source === "auto" ? "Auto" : "Manual"}
                    </span>
                  </div>
                  <div className="text-muted dark:text-muted-dark">
                    {formatTime(s.t)} · {relativeTime(s.t)}
                  </div>
                </div>
              ))}
            </div>
          </div>
        </section>
      </div>

      <SnapshotTitleDialog
        open={dialogOpen}
        title="Apply Xray config"
        description="A snapshot is captured before the live Xray process is updated."
        defaultValue={changed ? "Apply pending stack changes" : "Re-apply current Xray config"}
        confirmLabel="Apply"
        confirmIcon={<Zap size={14} />}
        pending={apply.isPending}
        onCancel={() => setDialogOpen(false)}
        onConfirm={(value) =>
          apply.mutate(value, {
            onSuccess: () => {
              toast.show("Apply succeeded", "ok");
              setDialogOpen(false);
              plan.refetch();
              generated.refetch();
            },
            onError: (e: any) => toast.show(`Apply failed: ${e?.message ?? e}`, "bad"),
          })
        }
      />
    </>
  );
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-2">
      <span className="text-muted dark:text-muted-dark">{label}</span>
      <span className="text-right">{children}</span>
    </div>
  );
}
