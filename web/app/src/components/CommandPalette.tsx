import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useSummary } from '../api/hooks';
import { Search, X } from 'lucide-react';

type Item = { id: string; label: string; group: string; action: () => void };

export default function CommandPalette({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [q, setQ] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);
  const navigate = useNavigate();
  const { data: summary } = useSummary();

  useEffect(() => {
    if (open) {
      setQ('');
      setTimeout(() => inputRef.current?.focus(), 50);
    }
  }, [open]);

  const items = useMemo<Item[]>(() => {
    const list: Item[] = [
      { id: 'nav-overview', label: 'Overview', group: 'Navigate', action: () => navigate('/') },
      { id: 'nav-analytics', label: 'Analytics', group: 'Navigate', action: () => navigate('/analytics') },
      { id: 'nav-users', label: 'Users', group: 'Navigate', action: () => navigate('/users') },
      { id: 'nav-rules', label: 'Rules', group: 'Navigate', action: () => navigate('/rules') },
      { id: 'nav-routes', label: 'Routes', group: 'Navigate', action: () => navigate('/routes') },
      { id: 'nav-tunnels', label: 'Tunnels', group: 'Navigate', action: () => navigate('/tunnels') },
      { id: 'nav-logs', label: 'Logs', group: 'Navigate', action: () => navigate('/logs') },
      { id: 'nav-snapshots', label: 'Snapshots', group: 'Navigate', action: () => navigate('/snapshots') },
      { id: 'nav-settings', label: 'Settings', group: 'Navigate', action: () => navigate('/settings') },
    ];
    summary?.user_items?.forEach((u) =>
      list.push({ id: 'u-' + u.email, label: u.email, group: 'Users', action: () => navigate(`/users?q=${encodeURIComponent(u.email)}`) }),
    );
    summary?.direct_domains?.forEach((d) =>
      list.push({ id: 'd-' + d, label: d, group: 'Direct domains', action: () => navigate('/routes') }),
    );
    summary?.block_domains?.forEach((d) =>
      list.push({ id: 'b-' + d, label: d, group: 'Block rules', action: () => navigate('/rules') }),
    );
    return list;
  }, [summary, navigate]);

  const filtered = useMemo(() => {
    if (!q) return items.slice(0, 30);
    const ql = q.toLowerCase();
    return items.filter((i) => i.label.toLowerCase().includes(ql)).slice(0, 30);
  }, [q, items]);

  if (!open) return null;
  const grouped = filtered.reduce<Record<string, Item[]>>((acc, item) => {
    (acc[item.group] = acc[item.group] || []).push(item);
    return acc;
  }, {});

  return (
    <div className="fixed inset-0 z-50 grid place-items-start pt-24 px-4 bg-black/40" onClick={onClose}>
      <div className="w-full max-w-xl rounded-xl bg-panel dark:bg-panel-dark border border-border dark:border-border-dark shadow-elev mx-auto" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center gap-2 px-3 py-3 border-b border-border dark:border-border-dark">
          <Search size={16} className="text-muted" />
          <input
            ref={inputRef}
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Search anything…"
            className="flex-1 bg-transparent outline-none text-sm"
            onKeyDown={(e) => {
              if (e.key === 'Escape') onClose();
              if (e.key === 'Enter' && filtered[0]) { filtered[0].action(); onClose(); }
            }}
          />
          <button onClick={onClose} className="text-muted hover:text-text"><X size={16} /></button>
        </div>
        <div className="max-h-[60vh] overflow-y-auto p-2 text-sm">
          {Object.entries(grouped).map(([group, list]) => (
            <div key={group} className="mb-2">
              <div className="px-2 py-1 text-[11px] uppercase font-semibold text-muted tracking-wider">{group}</div>
              {list.map((item) => (
                <button
                  key={item.id}
                  onClick={() => { item.action(); onClose(); }}
                  className="block w-full text-left rounded-md px-3 py-1.5 hover:bg-bg dark:hover:bg-bg-dark"
                >
                  {item.label}
                </button>
              ))}
            </div>
          ))}
          {filtered.length === 0 && (
            <div className="text-muted dark:text-muted-dark px-3 py-6 text-center">No matches.</div>
          )}
        </div>
      </div>
    </div>
  );
}
