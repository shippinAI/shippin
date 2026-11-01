import { useQuery } from "@tanstack/react-query";
import { apiGet } from "../api/client";

export type InstanceUsage = { name: string; type: string; cpu_seconds: number; memory_bytes: number; disk_bytes: number; net_tx_bytes: number };
export type ProjectUsage = {
  project_id: string;
  instances: InstanceUsage[];
  cpu_seconds: number;
  memory_bytes: number;
  disk_bytes: number;
  net_tx_bytes: number;
  accrued_usd: number;
  monthly_usd: number;
};
export type Usage = { projects: ProjectUsage[]; rates: Record<string, number> };

/** Per-project resource use and cost from Incus metrics (refreshed every 30 s). */
export const useUsage = (tenantId: string) =>
  useQuery({
    queryKey: ["tenant", tenantId, "usage"],
    queryFn: () => apiGet<Usage>(`/v1/tenants/${tenantId}/usage`),
    enabled: !!tenantId,
    refetchInterval: 30_000,
    retry: false,
  });

const units = ["B", "KiB", "MiB", "GiB", "TiB"];
export function bytes(n: number) {
  let i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return `${n.toFixed(i ? 1 : 0)} ${units[i]}`;
}
