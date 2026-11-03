// Provider panel (/v1/operator/*) response shapes. No secrets are ever returned.
export type AdminTenant = {
  id: string; idp_org_id: string; name: string; plan_id: string; status: string; ssh_ip?: string; error?: string;
  created_at: string; owner: string; owner_email: string; subnet: string; router: string; workspace_count: number;
};

export type AdminWorkspace = {
  observation?: "observed" | "missing" | "unavailable";
  observed_at?: string; recorded_state?: string;
  id: string; tenant_id: string; tenant_name: string; tenant_status: string; name: string; state: string;
  ssh_port: number; ipv4?: string; created_by: string; created_at: string; host: string; url: string;
  public_ip: string; memory_bytes: number; error?: string;
};

export type AdminUser = {
  id: string; login: string; name: string; email: string; email_verified: boolean; kind: string;
  state: string; org_id: string; org: string; created_at: string;
};
export type AdminIdP = { id: string; name: string; type: string; state: string };
export type AdminApp = {
  id: string; name: string; project: string; project_id: string; client_id: string; app_type: string;
  auth_method: string; state: string; redirect_uris: string[]; grant_types: string[];
};
export type AdminIdentity = { users: AdminUser[]; idps: AdminIdP[]; apps: AdminApp[]; errors: string[]; issuer: string };

export type HostStats = {
  cpus: number; cpu_pct: number; load: [number, number, number]; mem_total: number; mem_available: number;
  uptime_s: number; zfs_pools: { name: string; size: number; alloc: number; free: number; health: string }[];
  instances: Record<string, number>; projects: number; ssh_ports: { min: number; max: number; used: number };
};
export type AdminHost = { label: string; stats: HostStats };

export type EdgeRoute = { id: string; server: string; hosts: string[]; upstreams: string[]; forward_auth: boolean };
export type EdgePort = { proto: string; port: number; dest: string; dport: number; source: string; owner: string };
export type CertInfo = { host: string; issuer: string; subjects: string[]; not_after: string; days_left: number; error?: string };
export type AdminEdge = {
  routes?: { items: EdgeRoute[] }; routes_error?: string;
  ports?: { public_ip: string; min: number; max: number; items: EdgePort[] }; ports_error?: string;
  certs: CertInfo[];
};
