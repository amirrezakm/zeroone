import clsx from 'clsx';

export default function StatusPill({ ok, label }: { ok: boolean; label: string }) {
  return (
    <span className={clsx('pill', ok ? 'pill-ok' : 'pill-bad')}>
      <span className="dot" />
      {label}
    </span>
  );
}
