import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Route, Routes } from "react-router-dom";
import Sidebar from "./components/Sidebar";
import Topbar from "./components/Topbar";
import { ToastProvider } from "./components/Toast";
import { useEventStream } from "./api/events";
import { useSummary } from "./api/hooks";
import { useMe } from "./api/auth";
import Overview from "./pages/Overview";
import Analytics from "./pages/Analytics";
import Users from "./pages/Users";
import Rules from "./pages/Rules";
import RoutesPage from "./pages/Routes";
import Tunnels from "./pages/Tunnels";
import Logs from "./pages/Logs";
import Snapshots from "./pages/Snapshots";
import XrayConfig from "./pages/XrayConfig";
import Plugins from "./pages/Plugins";
import Settings from "./pages/Settings";
import Login from "./pages/Login";

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
    const handler = () => qc.invalidateQueries({ queryKey: ["me"] });
    window.addEventListener("xray:auth-required", handler);
    return () => window.removeEventListener("xray:auth-required", handler);
  }, [qc]);

  // While the auth check is loading we show nothing rather than flashing
  // the login form on every reload of an already-authenticated session.
  if (me.isLoading) {
    return <div className="text-muted grid min-h-full place-items-center text-xs">Loading…</div>;
  }
  const data = me.data;
  const authed =
    !!data && (data.auth === "session" || (data.auth === "token" && !data.bootstrap_needed));
  if (!authed) {
    // In bootstrap mode (no admins yet) the form switches to
    // "create the first admin" and posts to /api/admins; otherwise it
    // posts to /api/login. Either way the dashboard stays hidden until
    // a session cookie exists — never render <Inner /> for an
    // unauthenticated caller.
    return (
      <Login
        bootstrapNeeded={!!data?.bootstrap_needed}
        onLoggedIn={() => {
          qc.invalidateQueries({ queryKey: ["me"] });
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
      <div className="flex min-w-0 flex-1 flex-col">
        <Topbar publicIP={summary?.public_ip} />
        <main className="flex-1 overflow-y-auto px-4 py-5 lg:px-6">
          <Routes>
            <Route path="/" element={<Overview />} />
            <Route path="/analytics" element={<Analytics />} />
            <Route path="/users" element={<Users />} />
            <Route path="/rules" element={<Rules />} />
            <Route path="/routes" element={<RoutesPage />} />
            <Route path="/tunnels" element={<Tunnels />} />
            <Route path="/logs" element={<Logs />} />
            <Route path="/snapshots" element={<Snapshots />} />
            <Route path="/xray-config" element={<XrayConfig />} />
            <Route path="/plugins" element={<Plugins />} />
            <Route path="/settings" element={<Settings />} />
          </Routes>
        </main>
      </div>
    </div>
  );
}
