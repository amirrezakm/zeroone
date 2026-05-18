import { useEffect } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Route, Routes } from 'react-router-dom';
import Sidebar from './components/Sidebar';
import Topbar from './components/Topbar';
import { ToastProvider } from './components/Toast';
import { useEventStream } from './api/events';
import { useSummary } from './api/hooks';
import { useMe } from './api/auth';
import Overview from './pages/Overview';
import Analytics from './pages/Analytics';
import Users from './pages/Users';
import Rules from './pages/Rules';
import RoutesPage from './pages/Routes';
import Tunnels from './pages/Tunnels';
import Logs from './pages/Logs';
import Snapshots from './pages/Snapshots';
import Plugins from './pages/Plugins';
import Settings from './pages/Settings';
import Login from './pages/Login';

export default function App() {
  return (
    <ToastProvider>
      <AuthGate />
    </ToastProvider>
  );
}

function AuthGate() {
  const me = useMe();
  const qc = useQueryClient();

  useEffect(() => {
    const handler = () => qc.invalidateQueries({ queryKey: ['me'] });
    window.addEventListener('xray:auth-required', handler);
    return () => window.removeEventListener('xray:auth-required', handler);
  }, [qc]);

  // While the auth check is loading we show nothing rather than flashing
  // the login form on every reload of an already-authenticated session.
  if (me.isLoading) {
    return <div className="min-h-full grid place-items-center text-xs text-muted">Loading…</div>;
  }
  const data = me.data;
  const authed = !!data && (data.auth === 'session' || (data.auth === 'token' && !data.bootstrap_needed));
  // Bootstrap (no admins yet) is treated as authed so the operator can
  // reach Settings → Admins via their existing Bearer cookie/header and
  // seed the first admin without being trapped on the login screen.
  if (data?.bootstrap_needed) {
    return <Inner />;
  }
  if (!authed) {
    return (
      <Login
        bootstrapNeeded={false}
        onLoggedIn={() => {
          qc.invalidateQueries({ queryKey: ['me'] });
        }}
      />
    );
  }
  return <Inner />;
}

function Inner() {
  const { data: summary } = useSummary();
  useEventStream();
  return (
    <div className="flex h-full">
      <Sidebar publicIP={summary?.public_ip} />
      <div className="flex-1 flex flex-col min-w-0">
        <Topbar publicIP={summary?.public_ip} />
        <main className="flex-1 overflow-y-auto px-4 lg:px-6 py-5">
          <Routes>
            <Route path="/" element={<Overview />} />
            <Route path="/analytics" element={<Analytics />} />
            <Route path="/users" element={<Users />} />
            <Route path="/rules" element={<Rules />} />
            <Route path="/routes" element={<RoutesPage />} />
            <Route path="/tunnels" element={<Tunnels />} />
            <Route path="/logs" element={<Logs />} />
            <Route path="/snapshots" element={<Snapshots />} />
            <Route path="/plugins" element={<Plugins />} />
            <Route path="/settings" element={<Settings />} />
          </Routes>
        </main>
      </div>
    </div>
  );
}
