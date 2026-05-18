import { ReactNode } from 'react';
import { ResponsiveContainer, AreaChart, Area } from 'recharts';

type Series = { t: number; v: number }[];

export default function KPICard({
  label,
  value,
  hint,
  series,
  tone = 'default',
  icon,
}: {
  label: string;
  value: ReactNode;
  hint?: ReactNode;
  series?: Series;
  tone?: 'default' | 'ok' | 'warn' | 'bad';
  icon?: ReactNode;
}) {
  const stroke = tone === 'ok' ? '#0a8050' : tone === 'warn' ? '#b45309' : tone === 'bad' ? '#b91c1c' : '#f38020';
  return (
    <div className="panel panel-pad relative overflow-hidden">
      <div className="flex items-start justify-between gap-2">
        <div className="kpi-label">{label}</div>
        {icon && <div className="text-muted dark:text-muted-dark">{icon}</div>}
      </div>
      <div className="mt-1 kpi-value">{value}</div>
      {hint && <div className="mt-1 kpi-foot">{hint}</div>}
      {series && series.length > 1 && (
        <div className="-mx-5 -mb-5 mt-3 h-12">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={series} margin={{ top: 0, right: 0, bottom: 0, left: 0 }}>
              <defs>
                <linearGradient id={`grad-${label}`} x1="0" x2="0" y1="0" y2="1">
                  <stop offset="0%" stopColor={stroke} stopOpacity={0.35} />
                  <stop offset="100%" stopColor={stroke} stopOpacity={0} />
                </linearGradient>
              </defs>
              <Area dataKey="v" stroke={stroke} strokeWidth={1.5} fill={`url(#grad-${label})`} isAnimationActive={false} />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      )}
    </div>
  );
}
