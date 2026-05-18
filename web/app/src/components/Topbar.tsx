import { useEffect, useState } from 'react';
import { LogOut, Moon, Search, Sun, Zap } from 'lucide-react';
import clsx from 'clsx';
import { useQueryClient } from '@tanstack/react-query';
import { useApplyPlan } from '../api/hooks';
import { logout, useMe } from '../api/auth';
import { useToast } from './Toast';
import CommandPalette from './CommandPalette';
import DiffModal from './DiffModal';

export default function Topbar({ publicIP }: { publicIP?: string }) {
  const [dark, setDark] = useState(() => document.documentElement.classList.contains('dark'));
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [diffOpen, setDiffOpen] = useState(false);
  const apply = useApplyPlan();
  const me = useMe();
  const qc = useQueryClient();
  const toast = useToast();

  async function doLogout() {
    try {
      await logout();
      qc.invalidateQueries({ queryKey: ['me'] });
      toast.show('Signed out', 'ok');
    } catch (e: any) {
      toast.show(`Logout failed: ${e?.message}`, 'bad');
    }
  }

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault();
        setPaletteOpen((v) => !v);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  function toggleTheme() {
    const next = !dark;
    setDark(next);
    document.documentElement.classList.toggle('dark', next);
    try { localStorage.setItem('xs-theme', next ? 'dark' : 'light'); } catch {}
  }

  const pending = apply.data?.changed === true;
  const allowed = apply.data?.allow_apply !== false;

  return (
    <>
      <header className="sticky top-0 z-30 flex items-center gap-3 px-4 lg:px-6 h-14 border-b border-border bg-panel/90 backdrop-blur dark:bg-panel-dark/90 dark:border-border-dark">
        <div className="md:hidden flex items-center gap-2">
          <div className="h-7 w-7 rounded-md bg-accent grid place-items-center">
            <span className="text-white text-xs font-bold">X</span>
          </div>
          <span className="font-semibold">{publicIP || 'Xray Stack'}</span>
        </div>
        <button
          onClick={() => setPaletteOpen(true)}
          className="hidden md:flex items-center gap-2 rounded-lg border border-border dark:border-border-dark bg-bg dark:bg-bg-dark px-3 py-1.5 text-sm text-muted dark:text-muted-dark hover:text-text dark:hover:text-text-dark min-w-[18rem]"
        >
          <Search size={14} />
          <span>Search users, rules, domains…</span>
          <kbd className="ml-auto text-[10px] font-mono bg-panel dark:bg-panel-dark border border-border dark:border-border-dark rounded px-1 py-0.5">⌘K</kbd>
        </button>
        <div className="flex-1" />
        {pending && (
          <button
            disabled={!allowed}
            onClick={() => setDiffOpen(true)}
            className={clsx('btn btn-primary', !allowed && 'btn-danger')}
          >
            <Zap size={14} />
            {allowed ? 'Review & deploy' : 'Apply locked'}
          </button>
        )}
        <button onClick={toggleTheme} className="btn" aria-label="Toggle theme">
          {dark ? <Sun size={14} /> : <Moon size={14} />}
        </button>
        {me.data?.auth === 'session' && (
          <button onClick={doLogout} className="btn" aria-label="Sign out" title={`Signed in as ${me.data.username}`}>
            <LogOut size={14} />
            <span className="hidden md:inline text-xs">{me.data.username}</span>
          </button>
        )}
      </header>
      <CommandPalette open={paletteOpen} onClose={() => setPaletteOpen(false)} />
      <DiffModal open={diffOpen} onClose={() => setDiffOpen(false)} />
    </>
  );
}
