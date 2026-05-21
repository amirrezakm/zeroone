import { NavLink } from "react-router-dom";
import {
  Activity,
  BarChart3,
  Box,
  FileCode2,
  FileLock,
  Layers,
  Puzzle,
  ScrollText,
  Settings,
  Shield,
  Users as UsersIcon,
} from "lucide-react";
import clsx from "clsx";

const REPO_URL = "https://github.com/amirrezakm/zeroone";

function GithubIcon({ size = 14 }: { size?: number }) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="currentColor"
      aria-hidden="true"
    >
      <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0 0 16 8c0-4.42-3.58-8-8-8z" />
    </svg>
  );
}

const NAV: { to: string; label: string; icon: typeof Activity }[] = [
  { to: "/", label: "Overview", icon: Activity },
  { to: "/analytics", label: "Analytics", icon: BarChart3 },
  { to: "/users", label: "Users", icon: UsersIcon },
  { to: "/rules", label: "Rules", icon: Shield },
  { to: "/routes", label: "Routes", icon: Layers },
  { to: "/tunnels", label: "Tunnels", icon: Box },
  { to: "/logs", label: "Logs", icon: ScrollText },
  { to: "/xray-config", label: "Xray Config", icon: FileCode2 },
  { to: "/snapshots", label: "Snapshots", icon: FileLock },
  { to: "/plugins", label: "Plugins", icon: Puzzle },
  { to: "/settings", label: "Settings", icon: Settings },
];

export default function Sidebar({ publicIP }: { publicIP?: string }) {
  return (
    <aside className="border-border bg-panel dark:border-border-dark dark:bg-panel-dark hidden shrink-0 flex-col border-r md:flex md:w-60 lg:w-64">
      <div className="border-border dark:border-border-dark flex items-center gap-2 border-b px-5 py-5">
        <div className="bg-accent grid h-8 w-8 place-items-center rounded-lg">
          <span className="text-sm font-bold text-white">Z</span>
        </div>
        <div className="leading-tight">
          <div className="text-sm font-bold tracking-tight">ZeroOne</div>
          <div className="text-muted dark:text-muted-dark text-xs">{publicIP || "—"}</div>
        </div>
      </div>
      <nav className="flex-1 space-y-0.5 overflow-y-auto p-3">
        {NAV.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === "/"}
            className={({ isActive }) => clsx("nav-item", isActive && "active")}
          >
            <span className="nav-mark" />
            <item.icon size={16} className="shrink-0" />
            <span>{item.label}</span>
          </NavLink>
        ))}
      </nav>
      <div className="border-border text-muted dark:border-border-dark dark:text-muted-dark flex items-center justify-between gap-2 border-t px-5 py-3 text-xs">
        <span className="font-mono tabular-nums">v{__APP_VERSION__}</span>
        <a
          href={REPO_URL}
          target="_blank"
          rel="noreferrer"
          aria-label="GitHub repository"
          title="GitHub repository"
          className="text-muted hover:text-text dark:text-muted-dark dark:hover:text-text-dark inline-flex items-center transition-colors"
        >
          <GithubIcon size={14} />
        </a>
      </div>
    </aside>
  );
}
