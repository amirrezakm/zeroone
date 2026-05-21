import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, post, put } from "./client";

export type XrayJob = {
  id: string;
  phase:
    | "queued"
    | "downloading"
    | "verifying"
    | "staging"
    | "swapping"
    | "restarting"
    | "done"
    | "failed";
  started_at: number;
  finished_at?: number;
  bytes_total?: number;
  bytes_done?: number;
  target_version?: string;
  source?: "online" | "upload";
  error?: string;
};

export type XrayResolved = {
  binary: string;
  assets_dir: string;
  source: "image" | "override";
};

export type XrayLatestAsset = { name: string; url: string; size: number };
export type XrayLatest = {
  tag_name: string;
  name?: string;
  published_at?: string;
  html_url?: string;
  assets?: XrayLatestAsset[];
};

export type XraySources = {
  release_base: string;
  release_api: string;
  release_mirror?: string;
  assets_mirror?: string;
};

export type XrayState = {
  installed_version?: string;
  installed_at?: number;
  source?: string;
  binary_sha256?: string;
  geoip_sha256?: string;
  geosite_sha256?: string;
  last_check?: number;
  last_check_latest?: string;
  previous_version?: string;
};

export type XrayStatus = {
  active: XrayResolved;
  active_version: string;
  image_version: string;
  state: XrayState;
  versions: string[];
  has_override: boolean;
  job?: XrayJob | null;
  last_job?: XrayJob | null;
  latest?: XrayLatest;
  sources: XraySources;
};

export type XrayStatusResponse = {
  ok: boolean;
  status: XrayStatus;
  allow_apply: boolean;
};

export type XrayUpdateConfigView = {
  release_mirror: string;
  assets_mirror: string;
  pinned_version: string;
  auto_check: boolean;
  include_geo: boolean;
  effective_sources?: XraySources;
};

export function useXrayStatus(opts?: { pollMs?: number }) {
  return useQuery({
    queryKey: ["xray-version"],
    queryFn: () => api<XrayStatusResponse>("/api/xray/version"),
    refetchInterval: opts?.pollMs ?? 5_000,
  });
}

export function useXrayUpdateConfig() {
  return useQuery({
    queryKey: ["xray-update-config"],
    queryFn: () => api<{ ok: boolean; config: XrayUpdateConfigView }>("/api/xray/update/config"),
  });
}

export function useCheckXrayLatest() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      api<{ ok: boolean; latest: XrayLatest; asset: string }>("/api/xray/version/check?force=1"),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["xray-version"] }),
  });
}

export function useStartXrayUpdate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (version?: string) =>
      post<{ ok: boolean; job: XrayJob }>("/api/xray/update", version ? { version } : undefined),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["xray-version"] }),
  });
}

export function useUploadXrayUpdate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (args: { file: File; sha256?: string; version?: string }) => {
      const fd = new FormData();
      fd.append("file", args.file);
      if (args.sha256) fd.append("sha256", args.sha256);
      if (args.version) fd.append("version", args.version);
      const resp = await fetch("/api/xray/update/upload", {
        method: "POST",
        body: fd,
        credentials: "include",
      });
      const text = await resp.text();
      let body: any = {};
      if (text) {
        try {
          body = JSON.parse(text);
        } catch {
          body = { error: text };
        }
      }
      if (!resp.ok) {
        throw new Error(body.error || `${resp.status} ${resp.statusText}`);
      }
      return body as { ok: boolean; job: XrayJob };
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["xray-version"] }),
  });
}

export function useRollbackXray() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => post<{ ok: boolean }>("/api/xray/rollback"),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["xray-version"] }),
  });
}

export function useResetXrayToImage() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => post<{ ok: boolean }>("/api/xray/reset-to-image"),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["xray-version"] }),
  });
}

export function useSaveXrayUpdateConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (patch: Partial<XrayUpdateConfigView>) =>
      put<{ ok: boolean; config: XrayUpdateConfigView }>("/api/xray/update/config", patch),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["xray-update-config"] });
      qc.invalidateQueries({ queryKey: ["xray-version"] });
    },
  });
}
