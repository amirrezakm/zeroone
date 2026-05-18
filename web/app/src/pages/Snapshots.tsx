import PageHeader from '../components/PageHeader';
import { useCreateSnapshot, useRollback, useSnapshots } from '../api/hooks';
import { useToast } from '../components/Toast';
import { formatTime, relativeTime } from '../lib/format';
import { Camera, RotateCcw } from 'lucide-react';

export default function Snapshots() {
  const { data } = useSnapshots();
  const rollback = useRollback();
  const create = useCreateSnapshot();
  const toast = useToast();
  const list = data?.snapshots ?? [];
  return (
    <>
      <PageHeader
        title="Snapshots"
        subtitle={`${list.length} captured · created automatically before every Xray apply`}
        actions={
          <button
            className="btn btn-primary"
            disabled={create.isPending}
            onClick={() => create.mutate(undefined, {
              onSuccess: () => toast.show('Snapshot captured', 'ok'),
              onError: (e: any) => toast.show(`Snapshot failed: ${e?.message}`, 'bad'),
            })}
          ><Camera size={14} /> {create.isPending ? 'Capturing…' : 'Snapshot now'}</button>
        }
      />
      <div className="panel">
        <div className="table-head grid grid-cols-[180px,180px,1fr,auto] px-4 py-2">
          <div>ID</div>
          <div>Captured</div>
          <div>Files</div>
          <div></div>
        </div>
        <div className="divide-y divide-border dark:divide-border-dark">
          {list.length === 0 && <div className="px-4 py-6 text-sm text-muted dark:text-muted-dark">No snapshots yet.</div>}
          {list.map((s) => (
            <div key={s.id} className="grid grid-cols-[180px,180px,1fr,auto] items-center px-4 py-3 text-sm">
              <div className="font-mono text-xs">{s.id}</div>
              <div className="text-xs">
                <div>{formatTime(s.t)}</div>
                <div className="text-muted dark:text-muted-dark">{relativeTime(s.t)}</div>
              </div>
              <div className="text-xs font-mono text-muted dark:text-muted-dark truncate">stack.json + xray.json</div>
              <div>
                <button
                  className="btn text-xs"
                  onClick={() => {
                    if (!confirm(`Rollback to snapshot ${s.id}? This overwrites stack.json + xray config and requires xray-stackd restart.`)) return;
                    rollback.mutate(s.id, {
                      onSuccess: () => toast.show('Rollback complete — restart xray-stackd to reload', 'warn'),
                      onError: (e: any) => toast.show(`Rollback failed: ${e?.message}`, 'bad'),
                    });
                  }}
                ><RotateCcw size={12} /> Rollback</button>
              </div>
            </div>
          ))}
        </div>
      </div>
    </>
  );
}
