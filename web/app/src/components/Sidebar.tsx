import { NavLink } from 'react-router-dom';
import {
  Activity,
  BarChart3,
  Box,
  FileLock,
  Layers,
  Puzzle,
  ScrollText,
  Settings,
  Shield,
  Users as UsersIcon,
} from 'lucide-react';
import clsx from 'clsx';

const NAV: { to: string; label: string; icon: typeof Activity }[] = [
  { to: '/', label: 'Overview', icon: Activity },
  { to: '/analytics', label: 'Analytics', icon: BarChart3 },
  { to: '/users', label: 'Users', icon: UsersIcon },
  { to: '/rules', label: 'Rules', icon: Shield },
  { to: '/routes', label: 'Routes', icon: Layers },
  { to: '/tunnels', label: 'Tunnels', icon: Box },
  { to: '/logs', label: 'Logs', icon: ScrollText },
  { to: '/snapshots', label: 'Snapshots', icon: FileLock },
  { to: '/plugins', label: 'Plugins', icon: Puzzle },
  { to: '/settings', label: 'Settings', icon: Settings },
];

export default function Sidebar({ publicIP }: { publicIP?: string }) {
  return (
    <aside className="hidden md:flex md:w-60 lg:w-64 shrink-0 flex-col border-r border-border bg-panel dark:border-border-dark dark:bg-panel-dark">
      <div className="flex items-center gap-2 px-5 py-5 border-b border-border dark:border-border-dark">
        <div className="h-8 w-8 rounded-lg bg-accent grid place-items-center">
          <span className="text-white text-sm font-bold">X</span>
        </div>
        <div className="leading-tight">
          <div className="text-sm font-bold tracking-tight">Xray Stack</div>
          <div className="text-xs text-muted dark:text-muted-dark">{publicIP || '—'}</div>
        </div>
      </div>
      <nav className="flex-1 overflow-y-auto p-3 space-y-0.5">
        {NAV.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === '/'}
            className={({ isActive }) => clsx('nav-item', isActive && 'active')}
          >
            <span className="nav-mark" />
            <item.icon size={16} className="shrink-0" />
            <span>{item.label}</span>
          </NavLink>
        ))}
      </nav>
      <div className="px-5 py-3 border-t border-border dark:border-border-dark text-xs text-muted dark:text-muted-dark">
        v0.2 · Cloudflare-style
      </div>
    </aside>
  );
}
