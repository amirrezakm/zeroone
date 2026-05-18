import { createContext, useCallback, useContext, useEffect, useState } from 'react';
import clsx from 'clsx';

type Tone = 'ok' | 'bad' | 'warn' | 'info';
type ToastItem = { id: number; text: string; tone: Tone };

const ToastContext = createContext<{ show: (text: string, tone?: Tone) => void } | null>(null);

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);
  const show = useCallback((text: string, tone: Tone = 'info') => {
    const id = Date.now() + Math.random();
    setItems((prev) => [...prev, { id, text, tone }]);
    setTimeout(() => setItems((prev) => prev.filter((t) => t.id !== id)), 4500);
  }, []);
  return (
    <ToastContext.Provider value={{ show }}>
      {children}
      <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2 max-w-sm">
        {items.map((t) => (
          <ToastView key={t.id} item={t} onDismiss={() => setItems((p) => p.filter((x) => x.id !== t.id))} />
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast() {
  const ctx = useContext(ToastContext);
  if (!ctx) return { show: (_t: string, _tone?: Tone) => {} };
  return ctx;
}

function ToastView({ item, onDismiss }: { item: ToastItem; onDismiss: () => void }) {
  const [show, setShow] = useState(false);
  useEffect(() => { setShow(true); }, []);
  return (
    <div
      className={clsx(
        'rounded-lg border bg-panel dark:bg-panel-dark shadow-elev px-3 py-2 text-sm flex items-center gap-2 transition-all',
        show ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-2',
        item.tone === 'ok' && 'border-ok/30 text-ok dark:text-ok-dark',
        item.tone === 'bad' && 'border-bad/30 text-bad dark:text-bad-dark',
        item.tone === 'warn' && 'border-warn/30 text-warn dark:text-warn-dark',
        item.tone === 'info' && 'border-border dark:border-border-dark',
      )}
      onClick={onDismiss}
    >
      {item.text}
    </div>
  );
}
