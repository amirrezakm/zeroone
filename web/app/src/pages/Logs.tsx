import { useEffect, useRef, useState } from 'react';
import { Pause, Play, Search, ScrollText, ShieldAlert } from 'lucide-react';
import PageHeader from '../components/PageHeader';
import { useAudit, useXrayLogs } from '../api/hooks';
import { formatTime } from '../lib/format';

type Tab = 'audit' | 'xray';

export default function Logs() {
  const [tab, setTab] = useState<Tab>('xray');
  return (
    <>
      <PageHeader title="Logs" subtitle="Live Xray journal and panel audit log." />
      <div className="panel">
        <div className="border-b border-border dark:border-border-dark px-3 py-2 flex gap-1">
          <TabButton active={tab === 'xray'} onClick={() => setTab('xray')} icon={<ScrollText size={14} />}>Live xray</TabButton>
          <TabButton active={tab === 'audit'} onClick={() => setTab('audit')} icon={<ShieldAlert size={14} />}>Audit</TabButton>
        </div>
        {tab === 'xray' ? <XrayLogsTab /> : <AuditTab />}
      </div>
    </>
  );
}

function TabButton({ active, onClick, icon, children }: { active: boolean; onClick: () => void; icon: React.ReactNode; children: React.ReactNode }) {
  return (
    <button
      className={`px-3 py-1.5 rounded-md text-sm font-medium flex items-center gap-1.5 ${active ? 'bg-bg dark:bg-bg-dark' : 'text-muted dark:text-muted-dark'}`}
      onClick={onClick}
    >{icon} {children}</button>
  );
}

function XrayLogsTab() {
  const [filter, setFilter] = useState('');
  const [paused, setPaused] = useState(false);
  const { data } = useXrayLogs(500, !paused);
  const containerRef = useRef<HTMLDivElement>(null);
  const stickToBottom = useRef(true);

  // Track whether the user is at the bottom; only auto-scroll if so.
  function onScroll() {
    const el = containerRef.current;
    if (!el) return;
    stickToBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
  }

  useEffect(() => {
    const el = containerRef.current;
    if (el && stickToBottom.current) el.scrollTop = el.scrollHeight;
  }, [data]);

  const lines = (data?.lines ?? []).filter((l) => !filter || l.toLowerCase().includes(filter.toLowerCase()));

  return (
    <>
      <div className="px-4 py-2 border-b border-border dark:border-border-dark flex items-center gap-2 text-xs">
        <Search size={12} className="text-muted" />
        <input
          className="input bg-transparent border-0 px-0 focus:ring-0 flex-1"
          placeholder="filter… (e.g. amirreza, apple, blocked)"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
        <span className="text-muted dark:text-muted-dark">{lines.length} / {data?.lines.length ?? 0} lines</span>
        <button className="btn text-xs" onClick={() => setPaused((p) => !p)}>
          {paused ? <><Play size={12} /> Resume</> : <><Pause size={12} /> Pause</>}
        </button>
      </div>
      <div
        ref={containerRef}
        onScroll={onScroll}
        className="font-mono text-[11px] leading-relaxed px-4 py-3 max-h-[60vh] overflow-auto whitespace-pre"
      >
        {lines.map((l, i) => (
          <div key={i} className={lineToneClass(l)}>{l}</div>
        ))}
        {lines.length === 0 && <div className="text-muted dark:text-muted-dark py-6 text-center font-sans">No matching log lines.</div>}
      </div>
    </>
  );
}

function lineToneClass(line: string): string {
  if (/error|failed|reject/i.test(line)) return 'text-bad dark:text-bad-dark';
  if (/warning|degraded/i.test(line)) return 'text-warn dark:text-warn-dark';
  if (/blocked|>> block/.test(line)) return 'text-muted dark:text-muted-dark';
  return '';
}

function AuditTab() {
  const audit = useAudit(500);
  const entries = audit.data?.entries ?? [];
  return (
    <>
      <div className="table-head grid grid-cols-[140px,1fr,160px,1fr] px-4 py-2">
        <div>Time</div>
        <div>Action</div>
        <div>Actor</div>
        <div>Detail</div>
      </div>
      <div className="divide-y divide-border dark:divide-border-dark max-h-[60vh] overflow-auto">
        {entries.length === 0 && (
          <div className="px-4 py-6 text-sm text-muted dark:text-muted-dark">No entries yet.</div>
        )}
        {entries.map((e, i) => (
          <div key={i} className="grid grid-cols-[140px,1fr,160px,1fr] px-4 py-2 text-sm items-start gap-2">
            <div className="text-xs text-muted dark:text-muted-dark whitespace-nowrap font-mono">{formatTime(e.t)}</div>
            <div>
              <span className="pill text-muted"><span className="dot" />{e.action}</span>
              {e.target && <span className="ml-2 font-mono text-xs">{e.target}</span>}
            </div>
            <div className="text-xs">{e.actor}</div>
            <div className="text-xs text-muted dark:text-muted-dark font-mono break-all">
              {e.data ? JSON.stringify(e.data) : ''}
            </div>
          </div>
        ))}
      </div>
    </>
  );
}
