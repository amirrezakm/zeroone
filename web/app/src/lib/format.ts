export function bytes(value: number): string {
  if (!isFinite(value) || value <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  let n = value;
  let i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return `${n.toFixed(i ? 1 : 0)} ${units[i]}`;
}

export function bps(value: number): string {
  if (!isFinite(value) || value <= 0) return '0 bps';
  const units = ['bps', 'kbps', 'Mbps', 'Gbps'];
  let n = value * 8;
  let i = 0;
  while (n >= 1000 && i < units.length - 1) {
    n /= 1000;
    i++;
  }
  return `${n.toFixed(i ? 1 : 0)} ${units[i]}`;
}

export function relativeTime(unixSeconds: number, now = Date.now()): string {
  const diff = Math.round(now / 1000) - unixSeconds;
  const abs = Math.abs(diff);
  const future = diff < 0;
  let n: number;
  let unit: string;
  if (abs < 60) { n = abs; unit = 's'; }
  else if (abs < 3600) { n = Math.floor(abs / 60); unit = 'm'; }
  else if (abs < 86400) { n = Math.floor(abs / 3600); unit = 'h'; }
  else { n = Math.floor(abs / 86400); unit = 'd'; }
  return future ? `in ${n}${unit}` : `${n}${unit} ago`;
}

// All UI timestamps are pinned to Iran time (UTC+3:30) regardless of the
// browser's local timezone, since the panel manages an Iran-hosted stack and
// operators need timestamps that line up with the server's journal.
export const PANEL_TIMEZONE = 'Asia/Tehran';

const longFmt = new Intl.DateTimeFormat('en-GB', {
  timeZone: PANEL_TIMEZONE,
  year: 'numeric', month: '2-digit', day: '2-digit',
  hour: '2-digit', minute: '2-digit', second: '2-digit',
  hour12: false,
});
const shortFmt = new Intl.DateTimeFormat('en-GB', {
  timeZone: PANEL_TIMEZONE,
  hour: '2-digit', minute: '2-digit', hour12: false,
});

export function formatTime(unixSeconds: number): string {
  return longFmt.format(new Date(unixSeconds * 1000));
}

export function formatTimeShort(unixSeconds: number): string {
  return shortFmt.format(new Date(unixSeconds * 1000));
}
