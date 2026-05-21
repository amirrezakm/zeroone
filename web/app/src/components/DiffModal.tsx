import { useEffect, useMemo, useState } from "react";
import { X, Zap } from "lucide-react";
import clsx from "clsx";
import { api } from "../api/client";
import { useApplyXray } from "../api/hooks";
import { useToast } from "./Toast";

export default function DiffModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [generated, setGenerated] = useState<any>(null);
  const [live, setLive] = useState<any>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [title, setTitle] = useState("Deploy generated config");
  const apply = useApplyXray();
  const toast = useToast();

  useEffect(() => {
    if (!open) return;
    setTitle("Deploy generated config");
    setLoading(true);
    setError(null);
    Promise.all([api<any>("/api/xray/generated"), api<any>("/api/xray/live")])
      .then(([g, l]) => {
        setGenerated(g);
        setLive(l);
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [open]);

  const diffText = useMemo(() => {
    if (!generated || !live) return "";
    const g = JSON.stringify(generated, null, 2);
    const l = JSON.stringify(live, null, 2);
    if (g === l) return "";
    return makeUnifiedDiff(l, g);
  }, [generated, live]);

  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-black/50 p-4" onClick={onClose}>
      <div
        className="panel flex max-h-[85vh] w-full max-w-4xl flex-col overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="border-border dark:border-border-dark flex items-center justify-between border-b px-5 py-3">
          <div>
            <h2 className="font-semibold">Deploy generated config</h2>
            <p className="text-muted dark:text-muted-dark text-xs">
              Review the diff between live Xray and the generated config before applying.
            </p>
          </div>
          <button onClick={onClose} className="btn px-2">
            <X size={14} />
          </button>
        </div>
        <div className="bg-bg dark:bg-bg-dark flex-1 overflow-auto p-4">
          {loading && <div className="text-muted dark:text-muted-dark p-4 text-sm">Loading…</div>}
          {error && <div className="text-bad dark:text-bad-dark p-4 text-sm">Error: {error}</div>}
          {!loading && !error && diffText === "" && (
            <div className="text-muted dark:text-muted-dark p-4 text-sm">
              No diff between generated and live. Apply will be a no-op.
            </div>
          )}
          {!loading && !error && diffText !== "" && (
            <pre className="font-mono text-xs leading-snug whitespace-pre-wrap">
              {diffText.split("\n").map((line, i) => {
                const cls = line.startsWith("+")
                  ? "text-ok dark:text-ok-dark bg-ok/5"
                  : line.startsWith("-")
                    ? "text-bad dark:text-bad-dark bg-bad/5"
                    : "";
                return (
                  <div key={i} className={clsx("px-2", cls)}>
                    {line || " "}
                  </div>
                );
              })}
            </pre>
          )}
        </div>
        <div className="border-border dark:border-border-dark flex items-center gap-2 border-t px-5 py-3">
          <label className="text-muted dark:text-muted-dark text-xs" htmlFor="diff-title">
            Snapshot title
          </label>
          <input
            id="diff-title"
            type="text"
            className="input flex-1"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            maxLength={120}
          />
          <button className="btn" onClick={onClose}>
            Cancel
          </button>
          <button
            className="btn btn-primary"
            disabled={apply.isPending || !title.trim()}
            onClick={() =>
              apply.mutate(title.trim(), {
                onSuccess: () => {
                  toast.show("Apply succeeded", "ok");
                  onClose();
                },
                onError: (e: any) => toast.show(`Apply failed: ${e?.message ?? e}`, "bad"),
              })
            }
          >
            <Zap size={14} /> Confirm deploy
          </button>
        </div>
      </div>
    </div>
  );
}

// Tiny unified-diff: line-by-line. Good enough for JSON config previews.
function makeUnifiedDiff(a: string, b: string): string {
  const al = a.split("\n");
  const bl = b.split("\n");
  const out: string[] = [];
  const max = Math.max(al.length, bl.length);
  for (let i = 0; i < max; i++) {
    const av = al[i] ?? "";
    const bv = bl[i] ?? "";
    if (av === bv) {
      out.push(" " + av);
    } else {
      if (av) out.push("-" + av);
      if (bv) out.push("+" + bv);
    }
  }
  return out.join("\n");
}
