export type Link = { name: string; url: string };

export type PortalURL = {
  host: string;   // base, e.g. https://edge.example.com
  portal: string; // <host>/me/<sub_token>
  sub: string;    // <host>/sub/<sub_token>
};

export type UserItem = {
  email: string;
  uuid: string;
  enabled: boolean;
  banned_until: number;
  quota_bytes: number;
  download_mbps: number;
  upload_mbps: number;
  bandwidth_port: number;
  daily_quota_bytes?: number;
  weekly_quota_bytes?: number;
  monthly_quota_bytes?: number;
  daily_reset_hhmm?: string;
  max_sessions?: number;
  used_daily_bytes?: number;
  used_weekly_bytes?: number;
  used_monthly_bytes?: number;
  daily_reset_at?: number;
  weekly_reset_at?: number;
  monthly_reset_at?: number;
  links: Link[];
  used_bytes?: number;
  last_seen?: number;
  last_ip?: string;
  sub_token?: string;
  portal_urls?: PortalURL[];
};

export type SocksItem = {
  name: string;
  listen: string;
  port: number;
  username: string;
  password: string;
  links: Link[];
};

export type TunnelSummary = {
  name: string;
  type: string;
  interface: string;
  systemd_unit?: string;
  priority: number;
};

export type ClientEndpoint = {
  name: string;
  host: string;
  port: number;
  network: 'ws' | 'xhttp';
  path: string;
  mode?: 'auto' | 'packet-up' | 'stream-up' | 'stream-one';
  tls: boolean;
  enabled: boolean;
};

export type ClientEndpointHealth = {
  name: string;
  host: string;
  network: string;
  url: string;
  enabled: boolean;
  ok: boolean;
  status_code?: number;
  latency_ms?: number;
  error?: string;
  landing_url?: string;
  landing_ok: boolean;
  landing_status?: number;
  checked_at: string;
};

export type Summary = {
  public_ip: string;
  client_endpoints: ClientEndpoint[];
  users: number;
  socks: number;
  allow_apply: boolean;
  user_items: UserItem[];
  socks_items: SocksItem[];
  direct_domains: string[];
  block_domains: string[];
  manual_blocks: string[];
  tunnels: TunnelSummary[];
  failover: { enabled: boolean; probe_ip: string; probe_port: number; cooldown_seconds: number };
};

export type TunnelHealth = {
  name: string;
  type: string;
  interface: string;
  systemd_unit: string;
  priority: number;
  up: boolean;
  healthy: boolean;
  ipv4?: string;
  probe?: string;
  latency_ms?: number;
  error?: string;
};

export type Health = {
  ok: boolean;
  generated_at: string;
  tunnels: TunnelHealth[];
};

export type Usage = {
  updated_at: number;
  users: { email: string; uplink: number; downlink: number; total: number }[];
};

export type SystemInfo = {
  cpu: { percent: number; detail: string };
  ram: { percent: number; detail: string; used_bytes: number; total_bytes: number };
  tunnels: {
    name: string;
    rx_bytes: number;
    tx_bytes: number;
    rx_dropped?: number;
    tx_dropped?: number;
    rx_errors?: number;
    tx_errors?: number;
  }[];
  updated_at: number;
};

export type FailoverMode = { outbound_tag: string; interface?: string };
export type FailoverModeName = 'auto' | 'manual' | 'preferred';
export type FailoverDecision = {
  checks: TunnelHealth[];
  decision: {
    current: FailoverMode;
    desired: FailoverMode;
    effective: FailoverMode;
    pending: boolean;
    confirmation_count: number;
    cooldown_remaining_seconds?: number;
    reason: string;
  };
  mode: FailoverModeName;
  preferred_tunnel?: string;
};

export type ApplyPlan = {
  ok: boolean;
  valid: boolean;
  changed: boolean;
  config_path: string;
  allow_apply: boolean;
  error?: string;
};

export type QuotaPlan = {
  generated_at: number;
  actions: { email: string; used_bytes: number; quota_bytes: number; action: string; reason: string }[];
};

export type BandwidthPlan = {
  device: string;
  limits: { email: string; port: number; download_mbps: number; upload_mbps: number }[];
  needs_apply: boolean;
  apply_locked: boolean;
  tc_commands: string[];
};

export type MetricSample = {
  t: number;
  v: Record<string, number>;
};

export type MetricsResponse = {
  ok: boolean;
  samples: MetricSample[];
  step_seconds: number;
  since: number;
};

export type AuditEntry = {
  t: number;
  actor: string;
  action: string;
  target?: string;
  data?: Record<string, unknown>;
};

export type SnapshotInfo = {
  id: string;
  t: number;
  stack_path: string;
  xray_path: string;
};

export type RelaySite = {
  domain: string;
  enabled: boolean;
  note?: string;
};

export type RelayDefaults = {
  listen: string;
  socks_port: number;
  outbound_tag: string;
  binary: string;
  config_path: string;
  health_probe: string;
  google_ip: string;
  front_domain: string;
  log_level: string;
};

export type RelayConfigView = {
  enabled: boolean;
  listen: string;
  socks_port: number;
  auth_key_set: boolean;
  script_url: string;
  deployment_ids: string[];
  google_ip: string;
  front_domain: string;
  log_level: string;
  binary: string;
  config_path: string;
  systemd_unit: string;
  outbound_tag: string;
  sites: RelaySite[];
  inbound_tags: string[];
  hosts_map: Record<string, string> | null;
  health_probe: string;
  notes: string;
  defaults: RelayDefaults;
};

export type RelayStatus = {
  enabled: boolean;
  managed: boolean;
  listen: string;
  outbound_tag: string;
  binary_path: string;
  config_path: string;
  systemd_unit?: string;
  pid?: number;
  running: boolean;
  listen_ok: boolean;
  health_ok: boolean;
  last_probe?: string;
  last_error?: string;
  latency_ms?: number;
  binary_found: boolean;
  enabled_sites: number;
  total_sites: number;
  restart_count: number;
  started_at?: string;
  generated_at?: string;
};

export type RelayLogEntry = {
  t: number;
  level: string;
  msg: string;
  err?: string;
};

export type RelayProbeResult = {
  ok: boolean;
  listen_ok: boolean;
  health_ok: boolean;
  latency_ms?: number;
  status?: string;
  error?: string;
  target?: string;
};
