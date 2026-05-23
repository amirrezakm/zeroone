import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { Zap, RefreshCw, Pencil, Eye, AlignLeft, Undo2, AlertTriangle } from "lucide-react";
import CodeMirror from "@uiw/react-codemirror";
import { json } from "@codemirror/lang-json";
import { githubLight, githubDark } from "@uiw/codemirror-theme-github";
import PageHeader from "../components/PageHeader";
import SnapshotTitleDialog from "../components/SnapshotTitleDialog";
import { useToast } from "../components/Toast";
import {
  useApplyPlan,
  useApplyRawXray,
  useApplyXray,
  useGeneratedXray,
  useLiveXray,
  useSnapshots,
} from "../api/hooks";
import { useDarkMode } from "../lib/useDarkMode";
import { formatTime, relativeTime } from "../lib/format";

// XrayConfig is the single hub for inspecting, editing, and re-applying the
// rendered xray config. By default it mirrors the daemon-rendered config
// (read-only) and the Apply button pushes pending stack changes. Edit mode
// lets an operator hand-edit xray.json and push it straight to the live
// process — useful for one-off tweaks, but transient: the next stack apply
// regenerates xray.json from stack.json and overwrites the edit.
export default function XrayConfig() {
  const generated = useGeneratedXray();
  const live = useLiveXray();
  const plan = useApplyPlan();
  const snapshots = useSnapshots();
  const apply = useApplyXray();
  const applyRaw = useApplyRawXray();
  const toast = useToast();
  const dark = useDarkMode();
  const [stackDialogOpen, setStackDialogOpen] = useState(false);
  const [rawDialogOpen, setRawDialogOpen] = useState(false);
  const [editMode, setEditMode] = useState(false);
  const [editValue, setEditValue] = useState("");
  // Snapshot of the live config the edit buffer was seeded from. Dirty/Revert
  // compare against this, never against the stack-rendered config, so applying
  // an edit can't silently clobber live-only drift.
  const [liveBaseline, setLiveBaseline] = useState("");
  const [enteringEdit, setEnteringEdit] = useState(false);

  const generatedText = useMemo(
    () => (generated.data ? JSON.stringify(generated.data, null, 2) : ""),
    [generated.data],
  );

  // In view mode the editor mirrors the daemon-rendered config (which
  // auto-refetches). In edit mode we leave the buffer alone so a background
  // refetch never clobbers in-progress edits.
  useEffect(() => {
    if (!editMode) setEditValue(generatedText);
  }, [generatedText, editMode]);

  const allowApply = plan.data?.allow_apply ?? false;
  const changed = plan.data?.changed ?? false;
  const dirty = editMode && editValue !== liveBaseline;
  const recent = (snapshots.data?.snapshots ?? []).slice(0, 5);

  const editorTheme = dark ? githubDark : githubLight;

  // Edit mode targets the live xray.json, so seed the buffer from the actual
  // on-disk config — not the stack render, which may have drifted. TanStack
  // Query's refetch resolves with an error state instead of throwing, so we
  // inspect the result and abort (staying in view mode) on failure rather than
  // falling back to the rendered config and risking a clobbering apply.
  async function enterEditMode() {
    setEnteringEdit(true);
    try {
      const res = await live.refetch();
      if (res.error || res.data == null) {
        const msg = (res.error as any)?.message ?? "live config unavailable";
        toast.show(`Failed to load live config: ${msg}`, "bad");
        return;
      }
      const seed = JSON.stringify(res.data, null, 2);
      setLiveBaseline(seed);
      setEditValue(seed);
      setEditMode(true);
    } finally {
      setEnteringEdit(false);
    }
  }

  function format() {
    try {
      setEditValue(JSON.stringify(JSON.parse(editValue), null, 2));
    } catch (e: any) {
      toast.show(`Invalid JSON: ${e?.message ?? e}`, "bad");
    }
  }

  function openRawApply() {
    try {
      JSON.parse(editValue);
    } catch (e: any) {
      toast.show(`Invalid JSON: ${e?.message ?? e}`, "bad");
      return;
    }
    setRawDialogOpen(true);
  }

  return (
    <>
      <PageHeader
        title="Xray Config"
        subtitle="View, edit, and apply the rendered xray.json. Every apply takes a titled snapshot."
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
              className="btn"
              disabled={enteringEdit}
              onClick={() => (editMode ? setEditMode(false) : enterEditMode())}
              title={
                editMode ? "Switch back to read-only view" : "Edit the live xray.json directly"
              }
            >
              {editMode ? <Eye size={14} /> : <Pencil size={14} />}
              {editMode ? "View" : "Edit"}
            </button>
            {editMode ? (
              <button
                className="btn btn-primary"
                disabled={!allowApply || !dirty || applyRaw.isPending}
                title={
                  !allowApply
                    ? "Apply is disabled — start zeroone with -allow-apply"
                    : !dirty
                      ? "No edits to apply"
                      : "Validate and write the edited config to live Xray"
                }
                onClick={openRawApply}
              >
                <Zap size={14} /> Apply edited config
              </button>
            ) : (
              <button
                className="btn btn-primary"
                disabled={!allowApply || apply.isPending}
                title={
                  allowApply
                    ? "Apply pending stack changes to live Xray"
                    : "Apply is disabled — start zeroone with -allow-apply"
                }
                onClick={() => setStackDialogOpen(true)}
              >
                <Zap size={14} /> Apply
              </button>
            )}
          </>
        }
      />

      <div className="grid gap-4 lg:grid-cols-[2fr_1fr]">
        <section className="panel">
          <div className="border-border dark:border-border-dark flex items-center justify-between border-b px-4 py-2">
            <div className="text-sm font-medium">
              {editMode ? "Edit live xray.json" : "Generated xray.json"}
            </div>
            <div className="flex items-center gap-3">
              {editMode && (
                <>
                  <button
                    className="text-muted dark:text-muted-dark inline-flex items-center gap-1 text-xs hover:underline"
                    onClick={format}
                    title="Reformat JSON"
                  >
                    <AlignLeft size={12} /> Format
                  </button>
                  <button
                    className="text-muted dark:text-muted-dark inline-flex items-center gap-1 text-xs hover:underline disabled:opacity-50"
                    onClick={() => setEditValue(liveBaseline)}
                    disabled={!dirty}
                    title="Discard edits and reload the live config"
                  >
                    <Undo2 size={12} /> Revert
                  </button>
                </>
              )}
              <div className="text-muted dark:text-muted-dark text-xs">
                {plan.isLoading
                  ? "checking plan…"
                  : dirty
                    ? "unsaved edits"
                    : changed
                      ? "pending changes vs live"
                      : "in sync with live"}
              </div>
            </div>
          </div>

          {editMode && (
            <div className="border-border dark:border-border-dark text-warn-dark bg-warn/5 flex items-start gap-2 border-b px-4 py-2 text-xs">
              <AlertTriangle size={14} className="mt-0.5 shrink-0" />
              <span>
                Editing writes directly to the live xray.json. The daemon regenerates it from
                stack.json on the next stack apply, so manual edits here are temporary.
              </span>
            </div>
          )}

          <div className="bg-bg dark:bg-bg-dark max-h-[60vh] overflow-auto">
            {generated.isLoading && (
              <div className="text-muted dark:text-muted-dark p-3 text-sm">Loading…</div>
            )}
            {generated.isError && (
              <div className="text-bad dark:text-bad-dark p-3 text-sm">
                Failed to load generated config.
              </div>
            )}
            {!generated.isLoading && !generated.isError && (
              <CodeMirror
                value={editValue}
                theme={editorTheme}
                height="60vh"
                extensions={[json()]}
                editable={editMode}
                readOnly={!editMode}
                onChange={(v) => setEditValue(v)}
                basicSetup={{
                  lineNumbers: true,
                  foldGutter: true,
                  highlightActiveLine: editMode,
                  searchKeymap: true,
                }}
              />
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
        open={stackDialogOpen}
        title="Apply Xray config"
        description="A snapshot is captured before the live Xray process is updated."
        defaultValue={changed ? "Apply pending stack changes" : "Re-apply current Xray config"}
        confirmLabel="Apply"
        confirmIcon={<Zap size={14} />}
        pending={apply.isPending}
        onCancel={() => setStackDialogOpen(false)}
        onConfirm={(value) =>
          apply.mutate(value, {
            onSuccess: () => {
              toast.show("Apply succeeded", "ok");
              setStackDialogOpen(false);
              plan.refetch();
              generated.refetch();
            },
            onError: (e: any) => toast.show(`Apply failed: ${e?.message ?? e}`, "bad"),
          })
        }
      />

      <SnapshotTitleDialog
        open={rawDialogOpen}
        title="Apply edited Xray config"
        description="The edited config is validated with xray, snapshotted, then written to the live process."
        defaultValue="Apply hand-edited Xray config"
        confirmLabel="Apply"
        confirmIcon={<Zap size={14} />}
        pending={applyRaw.isPending}
        onCancel={() => setRawDialogOpen(false)}
        onConfirm={(value) => {
          let parsed: unknown;
          try {
            parsed = JSON.parse(editValue);
          } catch (e: any) {
            toast.show(`Invalid JSON: ${e?.message ?? e}`, "bad");
            return;
          }
          applyRaw.mutate(
            { config: parsed, title: value },
            {
              onSuccess: () => {
                toast.show("Edited config applied", "ok");
                setRawDialogOpen(false);
                setEditMode(false);
                plan.refetch();
                generated.refetch();
              },
              onError: (e: any) => toast.show(`Apply failed: ${e?.message ?? e}`, "bad"),
            },
          );
        }}
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
