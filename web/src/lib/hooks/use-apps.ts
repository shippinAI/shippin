import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import type { AppsView, OperatorApps } from "../api/apps-types";
import { toast } from "../design/components/toast/toast-queue";

const key = (id: string) => ["apps", id];

/** Install status for the tenant VM. Polls only while this hook is mounted. */
export function useTenantApps(tenantId: string | undefined) {
  return useQuery({
    queryKey: key(tenantId ?? ""),
    queryFn: () => apiGet<AppsView>(`/v1/tenants/${tenantId}/apps`),
    enabled: !!tenantId,
    refetchInterval: 15_000,
    retry: false,
  });
}

export function useCopyApp(tenantId: string) {
  return useMutation({
    mutationFn: async (app: string) => {
      if (!tenantId || !app) return;
      await apiSend<unknown>("POST", `/v1/tenants/${tenantId}/apps/${app}/copy`);
    },
  });
}

export function usePrepareDesktop(tenantId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => apiSend<{ status: string }>("POST", `/v1/tenants/${tenantId}/desktop/prepare`),
    onMutate: () => toast.info("Preparing the VM desktop…", "This installs the desktop session only. You still install Claude yourself."),
    onSuccess: () => {
      toast.success("Desktop prerequisites are in place");
      void qc.invalidateQueries({ queryKey: key(tenantId) });
    },
    onError: (e: Error) => toast.error("Desktop prerequisites failed", e.message),
  });
}

const opKey = (id: string) => ["admin", "apps", id];

export function useOperatorApps(id: string | undefined) {
  return useQuery({
    queryKey: opKey(id ?? ""),
    queryFn: () => apiGet<OperatorApps>(`/v1/operator/tenants/${id}/apps`),
    enabled: !!id,
    refetchInterval: 15_000,
    retry: false,
  });
}

export function useOperatorPrepare(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => apiSend<{ status: string }>("POST", `/v1/operator/tenants/${id}/desktop/prepare`),
    onMutate: () => toast.info("Preparing the VM desktop…"),
    onSuccess: () => {
      toast.success("Desktop prerequisites are in place");
      void qc.invalidateQueries({ queryKey: opKey(id) });
    },
    onError: (e: Error) => toast.error("Desktop prerequisites failed", e.message),
  });
}
