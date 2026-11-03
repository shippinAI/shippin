import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import type { AttachResult, RRKind, ServiceList } from "../api/rrflow-types";
import { toast } from "../design/components/toast/toast-queue";

const key = (t: string, p: string) => ["tenant", t, "projects", p, "services"];

export const useServices = (tenantId: string, projectId: string) =>
  useQuery({
    queryKey: key(tenantId, projectId),
    queryFn: () => apiGet<ServiceList>(`/v1/tenants/${tenantId}/projects/${projectId}/services`),
    refetchInterval: (q) => (q.state.data?.items.some((s) => s.state !== "ready" && s.state !== "error") ? 4000 : false),
  });

/** Runs rrflow/state on the tenant's private server and binds it to this devspace. */
export function useAttachRRFlow(tenantId: string, projectId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (workspaceId?: string) =>
      apiSend<AttachResult>("POST", `/v1/tenants/${tenantId}/projects/${projectId}/services/rrflow`, workspaceId ? { workspace_id: workspaceId } : {}),
    onSuccess: () => {
      toast.success("Attaching rrflow", "It starts on your private server and connects to this devspace.");
      void qc.invalidateQueries({ queryKey: key(tenantId, projectId) });
    },
    onError: (e) => toast.error("rrflow not attached", e.message),
  });
}

export const useRRFlowState = <T,>(tenantId: string, projectId: string, serviceId: string | undefined, kind: RRKind) =>
  useQuery({
    queryKey: [...key(tenantId, projectId), serviceId, kind],
    queryFn: () => apiGet<T>(`/v1/tenants/${tenantId}/projects/${projectId}/services/${serviceId}/rrflow/${kind}?limit=50`),
    enabled: !!serviceId,
    refetchInterval: 10_000,
  });
