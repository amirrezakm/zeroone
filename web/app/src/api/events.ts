import { useEffect } from 'react';
import { useQueryClient } from '@tanstack/react-query';

type Event = {
  t: number;
  kind: string;
  data?: Record<string, unknown>;
};

export function useEventStream(handler?: (ev: Event) => void) {
  const qc = useQueryClient();
  useEffect(() => {
    const url = (import.meta.env.VITE_API_BASE ?? '') + '/api/events';
    const es = new EventSource(url, { withCredentials: true });
    const onMessage = (e: MessageEvent) => {
      try {
        const ev = JSON.parse(e.data) as Event;
        if (handler) handler(ev);
        // Auto-invalidate hot queries on relevant events.
        switch (ev.kind) {
          case 'audit':
          case 'apply':
            qc.invalidateQueries({ queryKey: ['summary'] });
            qc.invalidateQueries({ queryKey: ['apply-plan'] });
            qc.invalidateQueries({ queryKey: ['audit'] });
            qc.invalidateQueries({ queryKey: ['snapshots'] });
            break;
          case 'failover':
            qc.invalidateQueries({ queryKey: ['failover'] });
            break;
        }
      } catch { /* ignore malformed events */ }
    };
    // Listen to all named SSE events.
    ['audit', 'apply', 'failover', 'tunnel', 'quota'].forEach(kind => {
      es.addEventListener(kind, onMessage as EventListener);
    });
    es.addEventListener('hello', () => { /* connected */ });
    es.onerror = () => {
      // browser will auto-reconnect; nothing to do
    };
    return () => es.close();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
}
