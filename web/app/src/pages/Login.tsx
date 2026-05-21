import { useState } from "react";
import { KeyRound, LogIn, ShieldAlert, UserPlus } from "lucide-react";
import { login } from "../api/auth";
import { post } from "../api/client";

export default function Login({
  onLoggedIn,
  bootstrapNeeded,
}: {
  onLoggedIn: () => void;
  bootstrapNeeded: boolean;
}) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    if (bootstrapNeeded) {
      if (password.length < 8) {
        setError("Password must be at least 8 characters");
        return;
      }
      if (password !== confirm) {
        setError("Passwords do not match");
        return;
      }
    }
    setPending(true);
    try {
      if (bootstrapNeeded) {
        await post("/api/admins", { username: username.trim(), password });
        // Auto-log-in with the credentials the operator just typed so
        // they aren't bounced through a second form.
        await login(username.trim(), password);
      } else {
        await login(username.trim(), password);
      }
      onLoggedIn();
    } catch (err: any) {
      setError(err?.message || (bootstrapNeeded ? "Could not create admin" : "Login failed"));
    } finally {
      setPending(false);
    }
  }

  const title = bootstrapNeeded ? "Create the first admin" : "Sign in to the control panel";
  const buttonLabel = bootstrapNeeded
    ? pending
      ? "Creating…"
      : "Create admin"
    : pending
      ? "Signing in…"
      : "Sign in";
  const ButtonIcon = bootstrapNeeded ? UserPlus : LogIn;

  return (
    <div className="flex min-h-full items-center justify-center bg-bg p-6 dark:bg-bg-dark">
      <form onSubmit={submit} className="panel panel-pad w-full max-w-sm shadow-lg">
        <div className="mb-4 flex items-center gap-2">
          <div className="rounded-lg bg-accent/10 p-2 text-accent">
            <KeyRound size={18} />
          </div>
          <div>
            <h1 className="text-lg font-semibold leading-tight">Xray Stack</h1>
            <p className="text-xs text-muted dark:text-muted-dark">{title}</p>
          </div>
        </div>

        {bootstrapNeeded && (
          <div className="mb-4 flex items-start gap-2 rounded-md border border-warn/40 bg-warn/10 p-3 text-xs">
            <ShieldAlert size={14} className="mt-0.5 shrink-0 text-warn" />
            <div>
              No admin accounts exist yet. Pick a username and password — this account will own
              the panel until you add more admins from Settings.
            </div>
          </div>
        )}

        <label className="mb-3 block">
          <div className="mb-1 text-xs text-muted dark:text-muted-dark">Username</div>
          <input
            className="input"
            autoFocus
            autoComplete="username"
            required
            value={username}
            onChange={(e) => setUsername(e.target.value)}
          />
        </label>
        <label className="mb-3 block">
          <div className="mb-1 text-xs text-muted dark:text-muted-dark">Password</div>
          <input
            className="input"
            type="password"
            autoComplete={bootstrapNeeded ? "new-password" : "current-password"}
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </label>
        {bootstrapNeeded && (
          <label className="mb-4 block">
            <div className="mb-1 text-xs text-muted dark:text-muted-dark">Confirm password</div>
            <input
              className="input"
              type="password"
              autoComplete="new-password"
              required
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
            />
          </label>
        )}

        {error && (
          <div className="mb-3 rounded border border-bad/30 bg-bad/5 p-2 text-xs text-bad dark:text-bad-dark">
            {error}
          </div>
        )}

        <button
          type="submit"
          className="btn btn-primary w-full justify-center"
          disabled={pending || !username || !password || (bootstrapNeeded && !confirm)}
        >
          <ButtonIcon size={14} /> {buttonLabel}
        </button>
      </form>
    </div>
  );
}
