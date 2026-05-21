import { useEffect, useMemo, useRef, useState } from "react";
import { AlertTriangle, Download, RefreshCw, RotateCcw, Save, Upload, Zap } from "lucide-react";
import { useToast } from "../components/Toast";
import { bytes, relativeTime } from "../lib/format";
import { useSummary } from "../api/hooks";
import {
  useCheckXrayLatest,
  useResetXrayToImage,
  useRollbackXray,
  useSaveXrayUpdateConfig,
  useStartXrayUpdate,
  useUploadXrayUpdate,
  useXrayStatus,
  useXrayUpdateConfig,
  type XrayJob,
  type XrayUpdateConfigView,
} from "../api/xrayInstall";

function isActiveJob(job?: XrayJob | null): boolean {
  if (!job) return false;
  return job.phase !== "done" && job.phase !== "failed";
}

function compareSemver(a: string, b: string): number {
  // Mirrors xrayinstall.CompareVersions on the server. Stops at the
  // first non-numeric token so RC builds compare equal to release.
  const norm = (s: string) =>
    (s.startsWith("v") ? s.slice(1) : s)
      .split(/[-+]/)[0]
      .split(".")
      .slice(0, 3)
      .map((n) => parseInt(n, 10) || 0);
  const pa = norm(a);
  const pb = norm(b);
  for (let i = 0; i < 3; i++) {
    const x = pa[i] ?? 0;
    const y = pb[i] ?? 0;
    if (x !== y) return x < y ? -1 : 1;
  }
  return 0;
}

export default function SettingsXrayPanel() {
  const { data: summary } = useSummary();
  const status = useXrayStatus({ pollMs: 4_000 });
  const updateCfg = useXrayUpdateConfig();
  const check = useCheckXrayLatest();
  const startUpdate = useStartXrayUpdate();
  const upload = useUploadXrayUpdate();
  const rollback = useRollbackXray();
  const reset = useResetXrayToImage();
  const saveCfg = useSaveXrayUpdateConfig();
  const toast = useToast();

  const allowApply = !!(status.data?.allow_apply ?? summary?.allow_apply);
  const st = status.data?.status;
  const job = st?.job ?? null;
  const lastJob = st?.last_job ?? null;
  const phase = job?.phase ?? lastJob?.phase;
  const active = isActiveJob(job);

  const upstreamLatest = st?.latest?.tag_name;
  const installed = st?.active_version || st?.image_version || "";
  const newer = upstreamLatest && installed ? compareSemver(installed, upstreamLatest) < 0 : false;

  // --- offline upload ---
  const [uploadFile, setUploadFile] = useState<File | null>(null);
  const [uploadSHA, setUploadSHA] = useState("");
  const [uploadVer, setUploadVer] = useState("");
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  async function doUpload() {
    if (!uploadFile) return;
    try {
      await upload.mutateAsync({
        file: uploadFile,
        sha256: uploadSHA || undefined,
        version: uploadVer || undefined,
      });
      toast.show("Upload accepted — installing", "ok");
      setUploadFile(null);
      setUploadSHA("");
      setUploadVer("");
      if (fileInputRef.current) fileInputRef.current.value = "";
    } catch (e: any) {
      toast.show(`Upload failed: ${e?.message}`, "bad");
    }
  }

  async function doCheck() {
    try {
      const res = await check.mutateAsync();
      toast.show(`Latest is ${res.latest.tag_name}`, "ok");
    } catch (e: any) {
      toast.show(`Check failed: ${e?.message}`, "bad");
    }
  }

  async function doUpdate(version?: string) {
    if (!allowApply) {
      toast.show("Updates require -allow-apply", "warn");
      return;
    }
    try {
      const res = await startUpdate.mutateAsync(version);
      toast.show(`Update started: ${res.job.id}`, "ok");
    } catch (e: any) {
      toast.show(`Update failed: ${e?.message}`, "bad");
    }
  }

  async function doRollback() {
    if (!confirm(`Roll back xray to the previous version?`)) return;
    try {
      await rollback.mutateAsync();
      toast.show("Rolled back", "ok");
    } catch (e: any) {
      toast.show(`Rollback failed: ${e?.message}`, "bad");
    }
  }

  async function doReset() {
    if (
      !confirm(
        `Reset xray to the image-baked binary (${st?.image_version || "default"})? Override files will be wiped.`,
      )
    )
      return;
    try {
      await reset.mutateAsync();
      toast.show("Reset — running image binary", "ok");
    } catch (e: any) {
      toast.show(`Reset failed: ${e?.message}`, "bad");
    }
  }

  return (
    <div className="panel lg:col-span-2">
      <div className="flex items-center justify-between border-b border-border px-5 py-3 dark:border-border-dark">
        <h2 className="flex items-center gap-2 text-sm font-semibold tracking-tight">
          <Zap size={14} /> Xray runtime
        </h2>
        <div className="flex items-center gap-2">
          <button
            className="btn text-xs"
            onClick={doCheck}
            disabled={check.isPending || active}
            title="Poll upstream for the latest release"
          >
            <RefreshCw size={12} className={check.isPending ? "animate-spin" : ""} />
            Check for updates
          </button>
        </div>
      </div>

      <div className="grid gap-0 lg:grid-cols-[1.2fr,1fr]">
        <div className="space-y-3 border-b border-border p-5 dark:border-border-dark lg:border-b-0 lg:border-r">
          <StatusRow label="Installed">
            <span className="font-mono text-sm">{installed || "—"}</span>
            <SourcePill source={st?.active?.source} />
          </StatusRow>
          <StatusRow label="Image-baked">
            <span className="font-mono text-xs text-muted dark:text-muted-dark">
              {st?.image_version || "—"}
            </span>
          </StatusRow>
          <StatusRow label="Latest upstream">
            <span className="font-mono text-sm">{upstreamLatest || "—"}</span>
            {newer && (
              <span className="pill pill-warn text-xs">
                <span className="dot" /> Update available
              </span>
            )}
            {st?.state?.last_check ? (
              <span className="text-xs text-muted dark:text-muted-dark">
                checked {relativeTime(st.state.last_check)}
              </span>
            ) : null}
          </StatusRow>
          <StatusRow label="Binary path">
            <code className="break-all font-mono text-xs text-muted dark:text-muted-dark">
              {st?.active?.binary}
            </code>
          </StatusRow>
          <StatusRow label="Assets dir">
            <code className="break-all font-mono text-xs text-muted dark:text-muted-dark">
              {st?.active?.assets_dir}
            </code>
          </StatusRow>
          {st?.state?.binary_sha256 && (
            <StatusRow label="Binary sha256">
              <code className="break-all font-mono text-xs text-muted dark:text-muted-dark">
                {st.state.binary_sha256.slice(0, 12)}…{st.state.binary_sha256.slice(-6)}
              </code>
            </StatusRow>
          )}

          <div className="flex flex-wrap gap-2 pt-1">
            <button
              className="btn btn-primary text-xs"
              disabled={!allowApply || active || !upstreamLatest}
              onClick={() => doUpdate(upstreamLatest)}
              title="Download + verify + swap atomically"
            >
              <Download size={12} /> Update to {upstreamLatest || "latest"}
            </button>
            {st?.has_override && (
              <>
                <button
                  className="btn text-xs"
                  disabled={!allowApply || active || (st?.versions?.length ?? 0) < 2}
                  onClick={doRollback}
                >
                  <RotateCcw size={12} /> Rollback
                </button>
                <button className="btn text-xs" disabled={!allowApply || active} onClick={doReset}>
                  Reset to image
                </button>
              </>
            )}
          </div>

          {(active || (phase && (phase === "done" || phase === "failed"))) && (
            <JobProgress job={job ?? lastJob ?? null} />
          )}
        </div>

        <div className="space-y-3 p-5">
          <div>
            <h3 className="kpi-label mb-2 flex items-center gap-2">
              <Upload size={12} /> Offline upload
            </h3>
            <p className="mb-2 text-xs text-muted dark:text-muted-dark">
              Upload an upstream <code className="font-mono">Xray-linux-*.zip</code>. The geo files
              bundled inside are installed alongside the binary.
            </p>
            <input
              ref={fileInputRef}
              type="file"
              accept=".zip"
              className="block w-full text-xs"
              onChange={(e) => setUploadFile(e.target.files?.[0] ?? null)}
            />
            <label className="kpi-label mt-3 block">SHA-256 (recommended)</label>
            <input
              className="input font-mono text-xs"
              placeholder="64-hex characters"
              value={uploadSHA}
              onChange={(e) => setUploadSHA(e.target.value.trim())}
            />
            <label className="kpi-label mt-3 block">Version label (optional)</label>
            <input
              className="input text-xs"
              placeholder="v25.2.0"
              value={uploadVer}
              onChange={(e) => setUploadVer(e.target.value.trim())}
            />
            <div className="mt-3 flex justify-end">
              <button
                className="btn btn-primary text-xs"
                onClick={doUpload}
                disabled={!uploadFile || !allowApply || active || upload.isPending}
              >
                <Upload size={12} /> Upload + install
              </button>
            </div>
          </div>
        </div>
      </div>

      <MirrorConfig
        cfg={updateCfg.data?.config}
        pending={saveCfg.isPending}
        allowApply={allowApply}
        onSave={async (patch) => {
          try {
            await saveCfg.mutateAsync(patch);
            toast.show("Saved", "ok");
          } catch (e: any) {
            toast.show(`Save failed: ${e?.message}`, "bad");
          }
        }}
      />

      {!allowApply && (
        <div className="flex items-center gap-2 border-t border-border px-5 py-2 text-xs text-warn dark:border-border-dark dark:text-warn-dark">
          <AlertTriangle size={12} />
          Updates and mirror changes require the daemon to be started with{" "}
          <code className="font-mono">-allow-apply</code>.
        </div>
      )}
    </div>
  );
}

function StatusRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="grid grid-cols-[8rem,1fr] items-center gap-3">
      <div className="kpi-label">{label}</div>
      <div className="flex flex-wrap items-center gap-2">{children}</div>
    </div>
  );
}

function SourcePill({ source }: { source?: string }) {
  if (!source) return null;
  const cls = source === "override" ? "pill-ok" : "pill-warn";
  return (
    <span className={`pill ${cls} text-xs`}>
      <span className="dot" /> {source}
    </span>
  );
}

function JobProgress({ job }: { job: XrayJob | null }) {
  if (!job) return null;
  const pct =
    job.bytes_total && job.bytes_done ? Math.min(100, (job.bytes_done / job.bytes_total) * 100) : 0;
  const finished = job.phase === "done" || job.phase === "failed";
  return (
    <div className="rounded-lg border border-border p-3 dark:border-border-dark">
      <div className="flex items-center justify-between text-xs">
        <span className="font-medium">
          {job.target_version ? `${job.target_version} · ` : ""}
          {job.phase}
        </span>
        <span className="font-mono text-muted dark:text-muted-dark">
          {job.bytes_done ? bytes(job.bytes_done) : ""}
          {job.bytes_total ? ` / ${bytes(job.bytes_total)}` : ""}
        </span>
      </div>
      {!finished && (
        <div className="mt-2 h-1.5 w-full overflow-hidden rounded bg-bg dark:bg-bg-dark">
          <div
            className="bg-primary h-full transition-[width] duration-200"
            style={{ width: `${pct}%` }}
          />
        </div>
      )}
      {job.error && (
        <div className="mt-2 break-all text-xs text-bad dark:text-bad-dark">{job.error}</div>
      )}
    </div>
  );
}

function MirrorConfig({
  cfg,
  pending,
  allowApply,
  onSave,
}: {
  cfg?: XrayUpdateConfigView;
  pending: boolean;
  allowApply: boolean;
  onSave: (patch: Partial<XrayUpdateConfigView>) => void | Promise<void>;
}) {
  const initial = useMemo(
    () => ({
      release_mirror: cfg?.release_mirror ?? "",
      assets_mirror: cfg?.assets_mirror ?? "",
      pinned_version: cfg?.pinned_version ?? "",
      auto_check: cfg?.auto_check ?? true,
      include_geo: cfg?.include_geo ?? true,
    }),
    [cfg],
  );
  const [form, setForm] = useState(initial);
  useEffect(() => setForm(initial), [initial]);

  return (
    <div className="border-t border-border p-5 dark:border-border-dark">
      <h3 className="kpi-label mb-2">Mirror configuration</h3>
      <p className="mb-3 text-xs text-muted dark:text-muted-dark">
        Override the upstream URLs panel-wide. Empty values fall back to the env defaults (
        <code className="font-mono">ZEROONE_XRAY_RELEASE_MIRROR</code>,{" "}
        <code className="font-mono">ZEROONE_XRAY_ASSETS_MIRROR</code>), then to GitHub.
      </p>
      <div className="grid gap-3 sm:grid-cols-2">
        <div>
          <label className="kpi-label">Release mirror URL</label>
          <input
            className="input font-mono text-xs"
            placeholder="https://mirror.example.com/Xray-core/releases/download"
            value={form.release_mirror}
            onChange={(e) => setForm({ ...form, release_mirror: e.target.value })}
          />
        </div>
        <div>
          <label className="kpi-label">Geo assets mirror URL</label>
          <input
            className="input font-mono text-xs"
            placeholder="https://mirror.example.com/v2fly"
            value={form.assets_mirror}
            onChange={(e) => setForm({ ...form, assets_mirror: e.target.value })}
          />
        </div>
        <div>
          <label className="kpi-label">Pinned version</label>
          <input
            className="input font-mono text-xs"
            placeholder="v25.1.30"
            value={form.pinned_version}
            onChange={(e) => setForm({ ...form, pinned_version: e.target.value })}
          />
        </div>
        <div className="flex flex-col gap-2 pt-5">
          <label className="inline-flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={form.auto_check}
              onChange={(e) => setForm({ ...form, auto_check: e.target.checked })}
            />
            Auto-check for updates
          </label>
          <label className="inline-flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={form.include_geo}
              onChange={(e) => setForm({ ...form, include_geo: e.target.checked })}
            />
            Update geo files alongside binary
          </label>
        </div>
      </div>
      <div className="mt-3 flex justify-end">
        <button
          className="btn btn-primary text-xs"
          disabled={pending || !allowApply}
          onClick={() => onSave(form)}
        >
          <Save size={12} /> Save mirror config
        </button>
      </div>
    </div>
  );
}
