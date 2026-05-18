import { useEffect, useMemo, useState } from 'react';
import { Activity, Globe, Play, Plus, RefreshCw, Save, Trash2, Wifi, WifiOff } from 'lucide-react';
import PageHeader from '../components/PageHeader';
import StatusPill from '../components/StatusPill';
import { useToast } from '../components/Toast';
import {
  useAddRelaySite,
  useDeleteRelaySite,
  useRelayConfig,
  useRelayLogs,
  useRelayRestart,
  useRelayStatus,
  useRelayTest,
  useUpdateRelayConfig,
  useUpdateRelaySite,
  type RelayConfigPatch,
} from '../api/hooks';
import type { RelayConfigView, RelaySite } from '../api/types';
import { relativeTime } from '../lib/format';

export default function Plugins() {
  const cfg = useRelayConfig();
  const status = useRelayStatus();
  const update = useUpdateRelayConfig();
  const toast = useToast();

  const data = cfg.data?.config;
  const stat = status.data?.status;
  const managed = status.data?.managed ?? false;

  const headline = data?.enabled
    ? stat?.running
      ? `Active · ${stat.enabled_sites}/${stat.total_sites} sites routed via ${stat.outbound_tag}`
      : `Enabled but offline · listen ${data.listen || data.defaults.listen}`
    : 'Disabled';

  function toggleEnabled() {
    if (!data) return;
    update.mutate(
      { enabled: !data.enabled },
      {
        onSuccess: () => toast.show(!data.enabled ? 'Relay enabled' : 'Relay disabled', 'ok'),
        onError: (e: any) => toast.show(`Save failed: ${e?.message}`, 'bad'),
      },
    );
  }

  return (
    <>
      <PageHeader
        title="Plugins"
        subtitle={cfg.isLoading ? 'Loading…' : headline}
        actions={
          data && (
            <button
              className={`btn ${data.enabled ? 'btn' : 'btn-primary'}`}
              onClick={toggleEnabled}
              disabled={update.isPending}
            >
              {data.enabled ? <WifiOff size={14} /> : <Wifi size={14} />}
              {data.enabled ? 'Disable' : 'Enable'}
            </button>
          )
        }
      />

      {data && (
        <div className="grid lg:grid-cols-3 gap-5">
          <div className="lg:col-span-2 space-y-5">
            <RelayStatusCard status={stat} cfg={data} managed={managed} />
            <RelaySitesPanel data={data} />
          </div>
          <div className="space-y-5">
            <RelayConfigForm data={data} />
            <RelayLogsPanel />
          </div>
        </div>
      )}
    </>
  );
}

function RelayStatusCard({ status, cfg, managed }: { status: any; cfg: RelayConfigView; managed: boolean }) {
  const test = useRelayTest();
  const restart = useRelayRestart();
  const toast = useToast();

  const running = status?.running;
  const listenOK = status?.listen_ok;
  const healthOK = status?.health_ok;
  const overallOK = !!(running && listenOK && healthOK);
  const label = !cfg.enabled
    ? 'Disabled'
    : !status
      ? 'Unknown'
      : healthOK
        ? 'Healthy'
        : listenOK
          ? 'Listening · upstream fail'
          : running
            ? 'Process up · listen fail'
            : 'Down';

  function runTest() {
    test.mutate(undefined, {
      onSuccess: (res) => {
        const p = res.probe;
        toast.show(
          p.ok ? `Relay OK · ${p.latency_ms ?? '?'}ms via ${p.target ?? (cfg.health_probe || cfg.defaults.health_probe)}` : `Relay failed: ${p.error || p.status}`,
          p.ok ? 'ok' : 'bad',
        );
      },
      onError: (e: any) => toast.show(`Probe failed: ${e?.message}`, 'bad'),
    });
  }

  function doRestart() {
    if (!managed) {
      toast.show('Supervisor not running — start xray-stackd with -manage-relay', 'warn');
      return;
    }
    restart.mutate(undefined, {
      onSuccess: () => toast.show('Relay restart triggered', 'ok'),
      onError: (e: any) => toast.show(`Restart failed: ${e?.message}`, 'bad'),
    });
  }

  return (
    <section className="panel">
      <div className="px-5 py-4 flex items-center justify-between border-b border-border dark:border-border-dark">
        <div>
          <h2 className="text-sm font-semibold tracking-tight flex items-center gap-2">
            <Activity size={14} /> MasterHttpRelayVPN
          </h2>
          <p className="text-xs text-muted dark:text-muted-dark">
            HTTP relay through Google Apps Script · outbound tag <code>{cfg.outbound_tag || cfg.defaults.outbound_tag}</code>
          </p>
        </div>
        <StatusPill ok={overallOK} label={label} />
      </div>
      <div className="px-5 py-4 grid grid-cols-2 md:grid-cols-4 gap-3 text-sm">
        <Stat label="Listen" value={status?.listen || cfg.listen || cfg.defaults.listen} mono />
        <Stat label="Sites routed" value={`${status?.enabled_sites ?? cfg.sites.filter((s) => s.enabled).length}/${cfg.sites.length}`} />
        <Stat
          label="Latency"
          value={status?.latency_ms != null ? `${status.latency_ms} ms` : '—'}
          mono
          tone={status?.health_ok ? 'ok' : status?.last_error ? 'bad' : 'muted'}
        />
        <Stat label="PID" value={status?.pid ? String(status.pid) : '—'} mono />
        <Stat label="Binary" value={status?.binary_found ? 'found' : 'missing'} tone={status?.binary_found ? 'ok' : 'bad'} />
        <Stat label="Managed" value={managed ? 'supervised' : 'external'} tone={managed ? 'ok' : 'muted'} />
        <Stat label="Restarts" value={String(status?.restart_count ?? 0)} mono />
        <Stat
          label="Last probe"
          value={status?.last_probe ? relativeTime(new Date(status.last_probe).getTime() / 1000) : '—'}
        />
      </div>
      {status?.last_error && (
        <div className="px-5 py-2 text-xs text-bad dark:text-bad-dark border-t border-border dark:border-border-dark font-mono break-all">
          {status.last_error}
        </div>
      )}
      <div className="px-5 py-3 border-t border-border dark:border-border-dark flex flex-wrap gap-2">
        <button className="btn btn-primary text-xs" onClick={runTest} disabled={test.isPending || !cfg.enabled}>
          <Play size={12} /> {test.isPending ? 'Probing…' : 'Run probe'}
        </button>
        <button className="btn text-xs" onClick={doRestart} disabled={restart.isPending || !managed}>
          <RefreshCw size={12} /> {restart.isPending ? 'Restarting…' : 'Restart'}
        </button>
      </div>
    </section>
  );
}

function Stat({ label, value, mono, tone }: { label: string; value: string; mono?: boolean; tone?: 'ok' | 'bad' | 'muted' }) {
  const colour = tone === 'ok'
    ? 'text-ok dark:text-ok-dark'
    : tone === 'bad'
      ? 'text-bad dark:text-bad-dark'
      : tone === 'muted'
        ? 'text-muted dark:text-muted-dark'
        : '';
  return (
    <div>
      <div className="kpi-label">{label}</div>
      <div className={`${mono ? 'font-mono text-xs' : 'text-sm'} ${colour}`.trim()}>{value}</div>
    </div>
  );
}

function RelaySitesPanel({ data }: { data: RelayConfigView }) {
  const add = useAddRelaySite();
  const upd = useUpdateRelaySite();
  const del = useDeleteRelaySite();
  const toast = useToast();
  const [domain, setDomain] = useState('');
  const [note, setNote] = useState('');
  const enabledCount = useMemo(() => data.sites.filter((s) => s.enabled).length, [data.sites]);

  function submit() {
    const clean = domain.trim();
    if (!clean) return;
    add.mutate(
      { domain: clean, enabled: true, note: note.trim() || undefined },
      {
        onSuccess: () => {
          setDomain('');
          setNote('');
          toast.show(`Added ${clean}`, 'ok');
        },
        onError: (e: any) => toast.show(`Add failed: ${e?.message}`, 'bad'),
      },
    );
  }

  function toggle(site: RelaySite) {
    upd.mutate(
      { ...site, enabled: !site.enabled },
      {
        onError: (e: any) => toast.show(`Update failed: ${e?.message}`, 'bad'),
      },
    );
  }

  function remove(site: RelaySite) {
    if (!confirm(`Remove ${site.domain} from the relay routing list?`)) return;
    del.mutate(site.domain, {
      onSuccess: () => toast.show(`Removed ${site.domain}`, 'ok'),
      onError: (e: any) => toast.show(`Remove failed: ${e?.message}`, 'bad'),
    });
  }

  return (
    <section className="panel">
      <div className="px-5 py-4 flex items-center justify-between border-b border-border dark:border-border-dark">
        <div>
          <h2 className="text-sm font-semibold tracking-tight flex items-center gap-2">
            <Globe size={14} /> Sites routed through relay
          </h2>
          <p className="text-xs text-muted dark:text-muted-dark">
            {enabledCount} of {data.sites.length} enabled · xray sends matching traffic to <code>{data.outbound_tag || data.defaults.outbound_tag}</code>
          </p>
        </div>
      </div>
      <div className="px-5 py-3 border-b border-border dark:border-border-dark grid grid-cols-[1fr,1fr,auto] gap-2 items-center">
        <input
          className="input text-xs"
          placeholder="example.com or domain:keyword"
          value={domain}
          onChange={(e) => setDomain(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && submit()}
        />
        <input
          className="input text-xs"
          placeholder="Optional note"
          value={note}
          onChange={(e) => setNote(e.target.value)}
        />
        <button className="btn btn-primary text-xs" disabled={!domain.trim() || add.isPending} onClick={submit}>
          <Plus size={12} /> Add
        </button>
      </div>
      {data.sites.length === 0 ? (
        <div className="px-5 py-8 text-sm text-muted dark:text-muted-dark text-center">
          No sites yet. Add a domain above (e.g. <code>example.com</code>, <code>domain:youtube.com</code>, <code>regexp:.*\.example\.com</code>).
        </div>
      ) : (
        <div className="divide-y divide-border dark:divide-border-dark">
          {data.sites.map((s) => (
            <div key={s.domain} className="px-5 py-2.5 grid grid-cols-[1fr,2fr,auto,auto] gap-3 items-center text-sm">
              <div className="font-mono text-xs break-all">{s.domain}</div>
              <div className="text-xs text-muted dark:text-muted-dark truncate">{s.note || '—'}</div>
              <label className="flex items-center gap-2 text-xs cursor-pointer select-none">
                <input
                  type="checkbox"
                  checked={s.enabled}
                  onChange={() => toggle(s)}
                  disabled={upd.isPending}
                />
                {s.enabled ? 'on' : 'off'}
              </label>
              <button className="btn text-xs" onClick={() => remove(s)} disabled={del.isPending}>
                <Trash2 size={12} />
              </button>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

function RelayConfigForm({ data }: { data: RelayConfigView }) {
  const update = useUpdateRelayConfig();
  const toast = useToast();
  const [form, setForm] = useState({
    listen: data.listen,
    auth_key: '',
    script_url: data.script_url,
    deployment_ids: data.deployment_ids.join(', '),
    inbound_tags: data.inbound_tags.join(', '),
    binary: data.binary,
    config_path: data.config_path,
    systemd_unit: data.systemd_unit,
    outbound_tag: data.outbound_tag,
    health_probe: data.health_probe,
    notes: data.notes,
  });

  // Re-sync when server data changes (e.g. after enable toggle).
  useEffect(() => {
    setForm({
      listen: data.listen,
      auth_key: '',
      script_url: data.script_url,
      deployment_ids: data.deployment_ids.join(', '),
      inbound_tags: data.inbound_tags.join(', '),
      binary: data.binary,
      config_path: data.config_path,
      systemd_unit: data.systemd_unit,
      outbound_tag: data.outbound_tag,
      health_probe: data.health_probe,
      notes: data.notes,
    });
  }, [data]);

  function save() {
    const patch: RelayConfigPatch = {
      listen: form.listen,
      script_url: form.script_url,
      deployment_ids: form.deployment_ids.split(',').map((s) => s.trim()).filter(Boolean),
      inbound_tags: form.inbound_tags.split(',').map((s) => s.trim()).filter(Boolean),
      binary: form.binary,
      config_path: form.config_path,
      systemd_unit: form.systemd_unit,
      outbound_tag: form.outbound_tag,
      health_probe: form.health_probe,
      notes: form.notes,
    };
    if (form.auth_key.trim()) patch.auth_key = form.auth_key.trim();
    update.mutate(patch, {
      onSuccess: () => {
        toast.show('Relay config saved', 'ok');
        setForm((f) => ({ ...f, auth_key: '' }));
      },
      onError: (e: any) => toast.show(`Save failed: ${e?.message}`, 'bad'),
    });
  }

  return (
    <section className="panel">
      <div className="px-5 py-4 border-b border-border dark:border-border-dark">
        <h2 className="text-sm font-semibold tracking-tight">Plugin settings</h2>
        <p className="text-xs text-muted dark:text-muted-dark">
          Apps Script credentials and binary path. AUTH_KEY is write-only — leave blank to keep existing.
        </p>
      </div>
      <div className="px-5 py-4 space-y-3 text-sm">
        <Field label={`Listen address (default ${data.defaults.listen})`}>
          <input className="input" value={form.listen} onChange={(e) => setForm({ ...form, listen: e.target.value })} placeholder={data.defaults.listen} />
        </Field>
        <Field label={`AUTH_KEY · ${data.auth_key_set ? 'currently set' : 'unset'}`}>
          <input
            className="input"
            type="password"
            autoComplete="new-password"
            value={form.auth_key}
            onChange={(e) => setForm({ ...form, auth_key: e.target.value })}
            placeholder={data.auth_key_set ? '••••• (leave blank to keep)' : 'paste auth_key'}
          />
        </Field>
        <Field label="Apps Script URL">
          <input
            className="input"
            value={form.script_url}
            onChange={(e) => setForm({ ...form, script_url: e.target.value })}
            placeholder="https://script.google.com/macros/s/AKfyc.../exec"
          />
        </Field>
        <Field label="Extra deployment IDs (comma-separated)">
          <input
            className="input"
            value={form.deployment_ids}
            onChange={(e) => setForm({ ...form, deployment_ids: e.target.value })}
            placeholder="AKfyc... , AKfyc..."
          />
        </Field>
        <Field label="Inbound tags routed through relay (comma-separated)">
          <input
            className="input"
            value={form.inbound_tags}
            onChange={(e) => setForm({ ...form, inbound_tags: e.target.value })}
            placeholder="managed-socks-relay"
          />
        </Field>
        <Field label={`Binary path (default ${data.defaults.binary})`}>
          <input className="input" value={form.binary} onChange={(e) => setForm({ ...form, binary: e.target.value })} placeholder={data.defaults.binary} />
        </Field>
        <Field label={`Generated config path (default ${data.defaults.config_path})`}>
          <input className="input" value={form.config_path} onChange={(e) => setForm({ ...form, config_path: e.target.value })} placeholder={data.defaults.config_path} />
        </Field>
        <Field label="systemd unit (optional — leave blank to spawn as child)">
          <input className="input" value={form.systemd_unit} onChange={(e) => setForm({ ...form, systemd_unit: e.target.value })} placeholder="mhrv-rs.service" />
        </Field>
        <Field label={`Outbound tag (default ${data.defaults.outbound_tag})`}>
          <input className="input" value={form.outbound_tag} onChange={(e) => setForm({ ...form, outbound_tag: e.target.value })} placeholder={data.defaults.outbound_tag} />
        </Field>
        <Field label={`Health probe target (default ${data.defaults.health_probe})`}>
          <input className="input" value={form.health_probe} onChange={(e) => setForm({ ...form, health_probe: e.target.value })} placeholder={data.defaults.health_probe} />
        </Field>
        <Field label="Notes">
          <textarea
            className="input"
            value={form.notes}
            onChange={(e) => setForm({ ...form, notes: e.target.value })}
            rows={2}
          />
        </Field>
      </div>
      <div className="px-5 py-3 border-t border-border dark:border-border-dark flex justify-end">
        <button className="btn btn-primary text-xs" onClick={save} disabled={update.isPending}>
          <Save size={12} /> {update.isPending ? 'Saving…' : 'Save settings'}
        </button>
      </div>
    </section>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <div className="kpi-label mb-1">{label}</div>
      {children}
    </label>
  );
}

function RelayLogsPanel() {
  const logs = useRelayLogs(200, true);
  const events = logs.data?.events ?? [];
  const lines = logs.data?.lines ?? [];
  return (
    <section className="panel">
      <div className="px-5 py-4 border-b border-border dark:border-border-dark">
        <h2 className="text-sm font-semibold tracking-tight">Recent activity</h2>
        <p className="text-xs text-muted dark:text-muted-dark">
          Supervisor events + tail of <code className="break-all">{logs.data?.path || 'relay.log'}</code>
        </p>
      </div>
      <div className="max-h-72 overflow-auto divide-y divide-border dark:divide-border-dark">
        {events.slice().reverse().slice(0, 30).map((e, i) => (
          <div key={`ev-${i}-${e.t}`} className="px-5 py-2 text-xs">
            <span className={
              e.level === 'error' ? 'text-bad dark:text-bad-dark' :
              e.level === 'warn' ? 'text-warn dark:text-warn-dark' :
              'text-muted dark:text-muted-dark'
            }>[{e.level}]</span>{' '}
            <span>{e.msg}</span>
            {e.err && <span className="text-bad dark:text-bad-dark"> — {e.err}</span>}
            <div className="text-muted dark:text-muted-dark font-mono">{relativeTime(e.t)}</div>
          </div>
        ))}
        {events.length === 0 && lines.length === 0 && (
          <div className="px-5 py-6 text-xs text-muted dark:text-muted-dark">
            No events yet. Enable the plugin and run a probe to populate.
          </div>
        )}
        {lines.slice().reverse().slice(0, 40).map((line, i) => (
          <div key={`ln-${i}`} className="px-5 py-1.5 text-xs font-mono text-muted dark:text-muted-dark break-all">
            {line}
          </div>
        ))}
      </div>
    </section>
  );
}
