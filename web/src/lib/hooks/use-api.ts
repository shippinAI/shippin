import { useQuery } from "@tanstack/react-query";
import { apiGet } from "../api/client";
import type { Me, PublicSettings, Services, TenantSettings } from "../api/types";

export const usePublicSettings = () =>
  useQuery({ queryKey: ["settings", "public"], queryFn: () => apiGet<PublicSettings>("/v1/settings/public"), staleTime: 300_000 });

export const useMe = () => useQuery({ queryKey: ["me"], queryFn: () => apiGet<Me>("/v1/me"), retry: false, staleTime: 60_000 });

export const useServices = (enabled = true) =>
  useQuery({
    queryKey: ["platform", "services"],
    queryFn: () => apiGet<Services>("/v1/platform/services"),
    refetchInterval: 15_000,
    enabled,
  });

export const useTenantSettings = (tenantId?: string) =>
  useQuery({
    queryKey: ["tenant", tenantId, "settings"],
    queryFn: () => apiGet<TenantSettings>(`/v1/tenants/${tenantId}/settings`),
    enabled: !!tenantId,
  });
