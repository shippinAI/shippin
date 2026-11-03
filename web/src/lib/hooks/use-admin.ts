import { useQuery } from "@tanstack/react-query";
import { apiGet } from "../api/client";
import type { StaffSummary } from "../api/team-types";
import type { AdminEdge, AdminHost, AdminIdentity, AdminTenant, AdminWorkspace, SystemTenant } from "../api/admin-types";

const q = <T,>(key: string, path: string, refetchInterval = 20_000) => () =>
  useQuery({ queryKey: ["admin", key], queryFn: () => apiGet<T>(path), refetchInterval, retry: false });

/** 200 when the caller holds any staff permission; 403 otherwise (enforced by the API). */
export const useAdminAccess = q<{ role: string; name: string; user_id: string; staff: StaffSummary }>("access", "/v1/operator/access", 0);
export const useAdminSystem = q<SystemTenant>("system", "/v1/operator/system");
export const useAdminTenants = q<{ items: AdminTenant[]; warning?: string }>("tenants", "/v1/operator/tenants");
export const useAdminWorkspaces = q<{ items: AdminWorkspace[] }>("workspaces", "/v1/operator/workspaces", 8_000);
export const useAdminIdentity = q<AdminIdentity>("identity", "/v1/operator/identity", 60_000);
export const useAdminHost = q<AdminHost>("host", "/v1/operator/host", 10_000);
export const useAdminEdge = q<AdminEdge>("edge", "/v1/operator/edge", 60_000);
