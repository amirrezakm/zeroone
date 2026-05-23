import { useState } from "react";
import PageHeader from "../components/PageHeader";
import SnapshotTitleDialog from "../components/SnapshotTitleDialog";
import { useCreateSnapshot, useRollback, useSnapshots } from "../api/hooks";
import { useToast } from "../components/Toast";
import { formatTime, relativeTime } from "../lib/format";
import { Camera, RotateCcw } from "lucide-react";
import type { SnapshotInfo } from "../api/types";

type Dialog = { kind: "create" } | { kind: "rollback"; snapshot: SnapshotInfo } | null;

export default function Snapshots() {
  const { data } = useSnapshots();
  const rollback = useRollback();
  const create = useCreateSnapshot();
  const toast = useToast();
  const list = data?.snapshots ?? [];
  const [dialog, setDialog] = useState<Dialog>(null);

  return (
    <>
      <PageHeader
        title="Snapshots"
        subtitle={`${list.length} captured · manual snapshots kept forever, auto snapshots cap at 50`}
        actions={
          <button
            className="btn btn-primary"
            disabled={create.isPending}
            onClick={() => setDialog({ kind: "create" })}
          >
            <Camera size={14} /> Snapshot now
          </button>
        }
      />
      <div className="panel">
        <div className="table-head grid grid-cols-[180px_90px_1fr_160px_auto] px-4 py-2">
          <div>ID</div>
          <div>Source</div>
          <div>Title</div>
          <div>Captured</div>
          <div></div>
        </div>
        <div className="divide-border dark:divide-border-dark divide-y">
          {list.length === 0 && (
            <div className="text-muted dark:text-muted-dark px-4 py-6 text-sm">
              No snapshots yet.
            </div>
          )}
          {list.map((s) => (
            <div
              key={s.id}
              className="grid grid-cols-[180px_90px_1fr_160px_auto] items-center px-4 py-3 text-sm"
            >
              <div className="font-mono text-xs">{s.id}</div>
              <div>
                <SourceBadge source={s.source} />
              </div>
              <div className="min-w-0">
                <div className="truncate">
                  {s.title || <em className="text-muted dark:text-muted-dark">untitled</em>}
                </div>
                {s.action && (
                  <div className="text-muted dark:text-muted-dark truncate text-xs">{s.action}</div>
                )}
              </div>
              <div className="text-xs">
                <div>{formatTime(s.t)}</div>
                <div className="text-muted dark:text-muted-dark">{relativeTime(s.t)}</div>
              </div>
              <div>
                <button
                  className="btn text-xs"
                  onClick={() => setDialog({ kind: "rollback", snapshot: s })}
                >
                  <RotateCcw size={12} /> Rollback
                </button>
              </div>
            </div>
          ))}
        </div>
      </div>

      <SnapshotTitleDialog
        open={dialog?.kind === "create"}
        title="Capture snapshot"
        description="A point-in-time copy of stack.json + the current xray config."
        defaultValue={`Manual snapshot ${new Date().toISOString().slice(0, 16).replace("T", " ")}`}
        confirmLabel="Capture"
        confirmIcon={<Camera size={14} />}
        pending={create.isPending}
        onCancel={() => setDialog(null)}
        onConfirm={(value) =>
          create.mutate(value, {
            onSuccess: () => {
              toast.show("Snapshot captured", "ok");
              setDialog(null);
            },
            onError: (e: any) => toast.show(`Snapshot failed: ${e?.message ?? e}`, "bad"),
          })
        }
      />

      <SnapshotTitleDialog
        open={dialog?.kind === "rollback"}
        title="Rollback to snapshot"
        description={
          dialog?.kind === "rollback"
            ? `Overwrites stack.json + xray config from snapshot ${dialog.snapshot.id}. A pre-rollback snapshot will be taken automatically.`
            : undefined
        }
        defaultValue={
          dialog?.kind === "rollback"
            ? `Rollback context: ${dialog.snapshot.title || dialog.snapshot.id}`
            : ""
        }
        confirmLabel="Rollback"
        confirmIcon={<RotateCcw size={14} />}
        pending={rollback.isPending}
        onCancel={() => setDialog(null)}
        onConfirm={(value) => {
          if (dialog?.kind !== "rollback") return;
          rollback.mutate(
            { id: dialog.snapshot.id, title: value },
            {
              onSuccess: () => {
                toast.show("Rollback complete — restart zeroone to reload", "warn");
                setDialog(null);
              },
              onError: (e: any) => toast.show(`Rollback failed: ${e?.message ?? e}`, "bad"),
            },
          );
        }}
      />
    </>
  );
}

function SourceBadge({ source }: { source?: string }) {
  const isAuto = source === "auto";
  const isManual = source === "manual";
  const label = isAuto ? "Auto" : isManual ? "Manual" : "Legacy";
  const cls = isAuto
    ? "bg-warn/10 text-warn-dark"
    : isManual
      ? "bg-ok/10 text-ok-dark"
      : "bg-bg-dark/20 text-muted dark:text-muted-dark";
  return (
    <span className={`inline-flex rounded px-2 py-0.5 text-xs font-medium ${cls}`}>{label}</span>
  );
}
