import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import type { AdminTenantDetail, SetupStatus } from "../api/setup-types";

/** Setup checklist: the tenant's own view, or the provider panel's read of any tenant. */
export function useSetup(tenantId: string, scope: "tenant" | "operator") {
  const path = scope === "tenant" ? `/v1/tenants/${tenantId}/setup` : `/v1/operator/tenants/${tenantId}/setup`;
  return useQuery({ queryKey: ["setup", scope, tenantId], queryFn: () => apiGet<SetupStatus>(path), refetchInterval: 15_000, retry: false });
}

/** Provisions rrflow/state for every project that has a devspace but no instance yet. */
export function useProvisionRRFlow(tenantId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => apiSend<{ queued_projects: string[] }>("POST", `/v1/tenants/${tenantId}/setup/rrflow`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["setup"] }),
  });
}

export function useAdminTenantDetail(tenantId: string) {
  return useQuery({ queryKey: ["admin", "tenant", tenantId], queryFn: () => apiGet<AdminTenantDetail>(`/v1/operator/tenants/${tenantId}`), retry: false });
}
