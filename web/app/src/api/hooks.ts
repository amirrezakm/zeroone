import { useQuery, useQueryClient, useMutation } from '@tanstack/react-query';
import { api, del, post, put } from './client';
import type {
  ApplyPlan,
  AuditEntry,
  BandwidthPlan,
  ClientEndpointHealth,
  FailoverDecision,
  Health,
  MetricsResponse,
  QuotaPlan,
  RelayConfigView,
  RelayLogEntry,
  RelayProbeResult,
  RelaySite,
  RelayStatus,
  SnapshotInfo,
  Summary,
  SystemInfo,
  Usage,
} from './types';

export function useSummary() {
  return useQuery({
    queryKey: ['summary'],
    queryFn: () => api<Summary>('/api/config/summary'),
    refetchInterval: 15_000,
  });
}

export function useHealth() {
  return useQuery({
    queryKey: ['health'],
    queryFn: () => api<Health>('/api/health'),
    refetchInterval: 10_000,
  });
}

export function useClientEndpointHealth() {
  return useQuery({
    queryKey: ['client-endpoint-health'],
    queryFn: () => api<{ ok: boolean; generated_at: string; endpoints: ClientEndpointHealth[] }>('/api/client-endpoints/health'),
    refetchInterval: 30_000,
  });
}

export function useSystem() {
  return useQuery({
    queryKey: ['system'],
    queryFn: () => api<SystemInfo>('/api/system'),
    refetchInterval: 5_000,
  });
}

export function useUsage() {
  return useQuery({
    queryKey: ['usage'],
    queryFn: () => api<Usage>('/api/usage'),
    refetchInterval: 30_000,
  });
}

export function useApplyPlan() {
  return useQuery({
    queryKey: ['apply-plan'],
    queryFn: () => api<ApplyPlan>('/api/xray/apply-plan'),
    refetchInterval: 20_000,
  });
}

export function useFailover() {
  return useQuery({
    queryKey: ['failover'],
    queryFn: () => api<FailoverDecision>('/api/failover/decision'),
    refetchInterval: 10_000,
  });
}

export function useMetrics(range: '1h' | '24h') {
  return useQuery({
    queryKey: ['metrics', range],
    queryFn: () => api<MetricsResponse>(`/api/metrics?range=${range}`),
    refetchInterval: range === '1h' ? 5_000 : 30_000,
  });
}

export function useAudit(limit = 200) {
  return useQuery({
    queryKey: ['audit', limit],
    queryFn: () => api<{ ok: boolean; entries: AuditEntry[] }>(`/api/audit?limit=${limit}`),
    refetchInterval: 15_000,
  });
}

export function useSnapshots() {
  return useQuery({
    queryKey: ['snapshots'],
    queryFn: () => api<{ ok: boolean; snapshots: SnapshotInfo[] }>('/api/snapshots'),
    refetchInterval: 60_000,
  });
}

export function useQuotaPlan() {
  return useQuery({
    queryKey: ['quota-plan'],
    queryFn: () => api<QuotaPlan>('/api/quota/plan'),
    refetchInterval: 60_000,
  });
}

export function useBandwidthPlan() {
  return useQuery({
    queryKey: ['bandwidth-plan'],
    queryFn: () => api<BandwidthPlan>('/api/bandwidth/plan'),
    refetchInterval: 60_000,
  });
}

export function useApplyXray() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => post('/api/xray/apply'),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['apply-plan'] });
      qc.invalidateQueries({ queryKey: ['summary'] });
      qc.invalidateQueries({ queryKey: ['snapshots'] });
      qc.invalidateQueries({ queryKey: ['audit'] });
    },
  });
}

export function useSyncUsage() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => post('/api/usage/sync'),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['usage'] }),
  });
}

export function useRollback() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => post(`/api/snapshots/rollback?id=${encodeURIComponent(id)}`),
    onSuccess: () => qc.invalidateQueries(),
  });
}

export type TokenView = { id: string; scope: string; created_at: number; last_used: number; hash_short: string };
export function useTokens() {
  return useQuery({
    queryKey: ['tokens'],
    queryFn: () => api<{ ok: boolean; tokens: TokenView[] }>('/api/tokens'),
    refetchInterval: 60_000,
  });
}

export function useGeneratedXray() {
  return useQuery({
    queryKey: ['generated-xray'],
    queryFn: () => api<any>('/api/xray/generated'),
    refetchInterval: 30_000,
  });
}

export function useLiveXray() {
  return useQuery({
    queryKey: ['live-xray'],
    queryFn: async () => {
      // Live config is not exposed directly; we use apply-plan to compare and fall back to generated
      return null as any;
    },
    enabled: false,
  });
}

export type OnlineUser = {
  email: string;
  connections: number;            // new "accepted" flows seen in window
  connections_per_min: number;    // rate normalised to per-minute
  active_sessions: number;        // currently established TCP attributable to this user — 0 for CDN-fronted setups
  last_seen: number;
  ips: string[];
  ip_details?: { ip: string; last_seen: number }[];
  recent_destinations: string[];
};
export type OnlineSnapshot = {
  window_seconds: number;
  generated_at: number;
  users: OnlineUser[];
  active_tcp_sessions: number;
  total_connections: number;
  active_by_port: Record<string, number>;
  unique_client_ips: number;
};
export function useOnline(seconds = 300) {
  return useQuery({
    queryKey: ['online', seconds],
    queryFn: () => api<OnlineSnapshot>(`/api/users/online?seconds=${seconds}`),
    refetchInterval: 15_000,
  });
}

export type TrafficResponse = {
  ok: boolean;
  updated_at: number;
  inbounds: Record<string, { uplink: number; downlink: number }>;
  outbounds: Record<string, { uplink: number; downlink: number }>;
  rates: Record<string, { uplink_bps?: number; downlink_bps?: number }>;
  inbound_rates: Record<string, { uplink_bps?: number; downlink_bps?: number }>;
};
export function useTraffic() {
  return useQuery({
    queryKey: ['traffic'],
    queryFn: () => api<TrafficResponse>('/api/xray/traffic'),
    refetchInterval: 30_000,
  });
}

export type NotificationsView = {
  webhook: { url: string; events: string[] | null; secret_set: boolean };
  telegram: { chat_id: string; events: string[] | null; bot_token_set: boolean };
};
export function useNotifications() {
  return useQuery({
    queryKey: ['notifications'],
    queryFn: () => api<{ ok: boolean; notifications: NotificationsView }>('/api/notifications'),
  });
}

export function useResetUsage() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => post('/api/usage/reset'),
    onSuccess: () => qc.invalidateQueries(),
  });
}

export function useApplyQuota() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => post('/api/quota/apply'),
    onSuccess: () => qc.invalidateQueries(),
  });
}

export function useApplyBandwidth() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => post('/api/bandwidth/apply'),
    onSuccess: () => qc.invalidateQueries(),
  });
}

export function useCreateSnapshot() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => post<{ ok: boolean; snapshot: SnapshotInfo }>('/api/snapshots'),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['snapshots'] }),
  });
}

export type XrayLogs = { ok: boolean; unit: string; lines: string[] };
export function useXrayLogs(lines = 300, enabled = true) {
  return useQuery({
    queryKey: ['xray-logs', lines],
    queryFn: () => api<XrayLogs>(`/api/xray/logs?lines=${lines}`),
    refetchInterval: enabled ? 3_000 : false,
    enabled,
  });
}

export type UserBandwidthResponse = {
  ok: boolean;
  updated_at: number;
  users: Record<string, { uplink_bps: number; downlink_bps: number }>;
};
export function useUserBandwidth() {
  return useQuery({
    queryKey: ['user-bandwidth'],
    queryFn: () => api<UserBandwidthResponse>('/api/users/bandwidth'),
    refetchInterval: 5_000,
  });
}

export type ConnectTestResult = {
  ok: boolean;
  status: string;
  duration_ms: number;
  error?: string;
};
export function useTestConnect() {
  return useMutation({
    mutationFn: (req: { route: string; target?: string; port?: number }) =>
      post<ConnectTestResult>('/api/test/connect', req),
  });
}

export type FailoverHistoryEntry = {
  t: number;
  from: { outbound_tag: string; interface?: string };
  to: { outbound_tag: string; interface?: string };
  reason: string;
  error?: string;
};
export function useFailoverHistory() {
  return useQuery({
    queryKey: ['failover-history'],
    queryFn: () => api<{ ok: boolean; entries: FailoverHistoryEntry[]; retention_hours: number }>('/api/failover/history'),
    refetchInterval: 30_000,
  });
}

export type SetFailoverModeReq = { mode: 'auto' | 'manual' | 'preferred'; preferred_tunnel?: string };
export type SetFailoverModeResp = {
  ok: boolean;
  mode: 'auto' | 'manual' | 'preferred';
  preferred_tunnel?: string;
  effective: { outbound_tag: string; interface?: string };
  applied: boolean;
};
export function useSetFailoverMode() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: SetFailoverModeReq) => put<SetFailoverModeResp>('/api/failover/mode', req),
    // Optimistic update: snapshot the current failover decision and patch
    // mode/preferred_tunnel instantly so the UI reflects the change before
    // the network round-trip (which can take seconds — xray restart).
    onMutate: async (req) => {
      await qc.cancelQueries({ queryKey: ['failover'] });
      const previous = qc.getQueryData<any>(['failover']);
      if (previous) {
        qc.setQueryData(['failover'], {
          ...previous,
          mode: req.mode,
          preferred_tunnel: req.preferred_tunnel ?? '',
        });
      }
      return { previous };
    },
    onError: (_err, _req, context) => {
      // Roll back to the snapshot on failure.
      if (context?.previous) qc.setQueryData(['failover'], context.previous);
    },
    onSettled: () => {
      // Refetch everything that might have changed once the apply finishes.
      qc.invalidateQueries({ queryKey: ['failover'] });
      qc.invalidateQueries({ queryKey: ['failover-history'] });
      qc.invalidateQueries({ queryKey: ['summary'] });
      qc.invalidateQueries({ queryKey: ['health'] });
      qc.invalidateQueries({ queryKey: ['apply-plan'] });
    },
  });
}

export type DestinationItem = { destination: string; requests: number };
export type DestinationsResponse = {
  ok: boolean;
  items: DestinationItem[];
  total: number;
  window: string;
  updated_at: number;
};
export function useTopDestinations(limit = 20) {
  return useQuery({
    queryKey: ['top-destinations', limit],
    queryFn: () => api<DestinationsResponse>(`/api/analytics/destinations?limit=${limit}`),
    refetchInterval: 30_000,
  });
}

export function useRelayConfig() {
  return useQuery({
    queryKey: ['relay-config'],
    queryFn: () => api<{ ok: boolean; config: RelayConfigView }>('/api/relay/config'),
  });
}

export function useRelayStatus() {
  return useQuery({
    queryKey: ['relay-status'],
    queryFn: () => api<{ ok: boolean; status: RelayStatus; managed: boolean }>('/api/relay/status'),
    refetchInterval: 5_000,
  });
}

export function useRelayLogs(lines = 200, enabled = true) {
  return useQuery({
    queryKey: ['relay-logs', lines],
    queryFn: () => api<{ ok: boolean; lines: string[]; events: RelayLogEntry[]; path: string }>(`/api/relay/logs?lines=${lines}`),
    refetchInterval: enabled ? 5_000 : false,
    enabled,
  });
}

export type RelayConfigPatch = Partial<{
  enabled: boolean;
  listen: string;
  socks_port: number;
  auth_key: string;
  script_url: string;
  deployment_ids: string[];
  google_ip: string;
  front_domain: string;
  log_level: string;
  binary: string;
  config_path: string;
  systemd_unit: string;
  outbound_tag: string;
  inbound_tags: string[];
  hosts_map: Record<string, string>;
  health_probe: string;
  notes: string;
}>;

export function useUpdateRelayConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (patch: RelayConfigPatch) =>
      put<{ ok: boolean; config: RelayConfigView }>('/api/relay/config', patch),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['relay-config'] });
      qc.invalidateQueries({ queryKey: ['relay-status'] });
      qc.invalidateQueries({ queryKey: ['apply-plan'] });
    },
  });
}

export function useAddRelaySite() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (site: RelaySite) =>
      post<{ ok: boolean; sites: RelaySite[] }>('/api/relay/sites', site),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['relay-config'] });
      qc.invalidateQueries({ queryKey: ['relay-status'] });
      qc.invalidateQueries({ queryKey: ['apply-plan'] });
    },
  });
}

export function useUpdateRelaySite() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (site: RelaySite) =>
      put<{ ok: boolean; sites: RelaySite[] }>('/api/relay/sites', site),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['relay-config'] });
      qc.invalidateQueries({ queryKey: ['relay-status'] });
      qc.invalidateQueries({ queryKey: ['apply-plan'] });
    },
  });
}

export function useDeleteRelaySite() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (domain: string) =>
      del<{ ok: boolean; sites: RelaySite[] }>(`/api/relay/sites?domain=${encodeURIComponent(domain)}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['relay-config'] });
      qc.invalidateQueries({ queryKey: ['relay-status'] });
      qc.invalidateQueries({ queryKey: ['apply-plan'] });
    },
  });
}

export function useRelayTest() {
  return useMutation({
    mutationFn: () => post<{ ok: boolean; probe: RelayProbeResult }>('/api/relay/test'),
  });
}

export function useRelayRestart() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => post<{ ok: boolean }>('/api/relay/restart'),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['relay-status'] }),
  });
}
