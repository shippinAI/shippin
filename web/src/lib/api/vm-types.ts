/** Tenant VM as reported by GET /v1/operator/tenants/{id}/vm (mirrors hostd VMView). */
export type VMContainer = {
  name: string;
  workspace: string;
  status: string;
  ipv4: string;
  memory_bytes: number;
  processes: number;
};

export type TenantVM = {
  mode: "vm" | "legacy";
  name: string;
  status: string;
  ipv4: string;
  inner_subnet: string;
  cpu_limit: string;
  cpu_usage_ns: number;
  memory_limit: string;
  memory_used_bytes: number;
  memory_total_bytes: number;
  root_size: string;
  data_size: string;
  data_used_bytes: number;
  data_total_bytes: number;
  containers: VMContainer[] | null;
};

export type VMAction = "start" | "stop" | "restart";
