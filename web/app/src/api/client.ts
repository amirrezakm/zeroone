const apiBase = (import.meta.env.VITE_API_BASE as string | undefined) ?? '';

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiBase}${path}`, {
    cache: 'no-store',
    credentials: 'include',
    ...init,
    headers: {
      'Accept': 'application/json',
      ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
      ...init?.headers,
    },
  });
  const text = await response.text();
  let body: any = {};
  if (text) {
    try { body = JSON.parse(text); } catch { body = { error: text }; }
  }
  if (!response.ok) {
    const err = new Error(body.error || `${response.status} ${response.statusText}`);
    (err as any).status = response.status;
    if (response.status === 401 && path !== '/api/login' && path !== '/api/me') {
      // Session expired or revoked. Surface via an event so the App can
      // bounce the user back to the login page without each call site
      // needing to know about auth.
      try {
        window.dispatchEvent(new CustomEvent('xray:auth-required'));
      } catch { /* SSR / no window */ }
    }
    throw err;
  }
  return body as T;
}

export const post = <T>(path: string, body?: unknown) =>
  api<T>(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined });
export const put = <T>(path: string, body?: unknown) =>
  api<T>(path, { method: 'PUT', body: body ? JSON.stringify(body) : undefined });
export const del = <T>(path: string) => api<T>(path, { method: 'DELETE' });
