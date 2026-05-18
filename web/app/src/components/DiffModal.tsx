import { useEffect, useMemo, useState } from 'react';
import { X, Zap } from 'lucide-react';
import clsx from 'clsx';
import { api } from '../api/client';
import { useApplyXray } from '../api/hooks';
import { useToast } from './Toast';

export default function DiffModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [generated, setGenerated] = useState<any>(null);
  const [live, setLive] = useState<any>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const apply = useApplyXray();
  const toast = useToast();

  useEffect(() => {
    if (!open) return;
    setLoading(true);
    setError(null);
    Promise.all([
      api<any>('/api/xray/generated'),
      api<any>('/api/xray/live'),
    ])
      .then(([g, l]) => { setGenerated(g); setLive(l); })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [open]);

  const diffText = useMemo(() => {
    if (!generated || !live) return '';
    const g = JSON.stringify(generated, null, 2);
    const l = JSON.stringify(live, null, 2);
    if (g === l) return '';
    return makeUnifiedDiff(l, g);
  }, [generated, live]);

  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 grid place-items-center p-4 bg-black/50" onClick={onClose}>
      <div className="panel w-full max-w-4xl max-h-[85vh] overflow-hidden flex flex-col" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between px-5 py-3 border-b border-border dark:border-border-dark">
          <div>
            <h2 className="font-semibold">Deploy generated config</h2>
            <p className="text-xs text-muted dark:text-muted-dark">Review the diff between live Xray and the generated config before applying.</p>
          </div>
          <button onClick={onClose} className="btn px-2"><X size={14} /></button>
        </div>
        <div className="flex-1 overflow-auto bg-bg dark:bg-bg-dark p-4">
          {loading && <div className="text-sm text-muted dark:text-muted-dark p-4">Loading…</div>}
          {error && <div className="text-sm text-bad dark:text-bad-dark p-4">Error: {error}</div>}
          {!loading && !error && diffText === '' && (
            <div className="text-sm text-muted dark:text-muted-dark p-4">No diff between generated and live. Apply will be a no-op.</div>
          )}
          {!loading && !error && diffText !== '' && (
            <pre className="text-xs font-mono whitespace-pre-wrap leading-snug">
              {diffText.split('\n').map((line, i) => {
                const cls = line.startsWith('+') ? 'text-ok dark:text-ok-dark bg-ok/5' : line.startsWith('-') ? 'text-bad dark:text-bad-dark bg-bad/5' : '';
                return <div key={i} className={clsx('px-2', cls)}>{line || ' '}</div>;
              })}
            </pre>
          )}
        </div>
        <div className="flex items-center justify-end gap-2 px-5 py-3 border-t border-border dark:border-border-dark">
          <button className="btn" onClick={onClose}>Cancel</button>
          <button
            className="btn btn-primary"
            disabled={apply.isPending}
            onClick={() => apply.mutate(undefined, {
              onSuccess: () => { toast.show('Apply succeeded', 'ok'); onClose(); },
              onError: (e: any) => toast.show(`Apply failed: ${e?.message ?? e}`, 'bad'),
            })}
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
  const al = a.split('\n');
  const bl = b.split('\n');
  const out: string[] = [];
  const max = Math.max(al.length, bl.length);
  for (let i = 0; i < max; i++) {
    const av = al[i] ?? '';
    const bv = bl[i] ?? '';
    if (av === bv) {
      out.push(' ' + av);
    } else {
      if (av) out.push('-' + av);
      if (bv) out.push('+' + bv);
    }
  }
  return out.join('\n');
}
