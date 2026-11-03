import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import type { EnrollToken, Server, ServerList } from "../api/server-types";
import { toast } from "../design/components/toast/toast-queue";

const key = (tenantId?: string) => ["servers", tenantId];

/** Servers of the org plus planned sites. Refreshes at the heartbeat interval the API reports. */
export const useServers = (tenantId?: string) =>
  useQuery({
    queryKey: key(tenantId),
    queryFn: () => apiGet<ServerList>(`/v1/tenants/${tenantId}/servers`),
    enabled: !!tenantId,
    retry: false,
    refetchInterval: (q) => (q.state.data?.heartbeat_seconds ?? 0) * 1000 || false,
  });

export function useCreateServer(tenantId?: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { slug: string; label: string; site: string }) => apiSend<Server>("POST", `/v1/tenants/${tenantId}/servers`, body),
    onSuccess: () => void qc.invalidateQueries({ queryKey: key(tenantId) }),
    onError: (e) => toast.error("Server not added", e.message),
  });
}

/** Issues a single-use token. The caller shows it once and must not cache or log it. */
export function useIssueToken(tenantId?: string) {
  return useMutation({
    mutationFn: (serverId: string) => apiSend<EnrollToken>("POST", `/v1/tenants/${tenantId}/servers/${serverId}/enroll-token`),
    onError: (e) => toast.error("No token issued", e.message),
  });
}

export function useRevokeServer(tenantId?: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (serverId: string) => apiSend<Server>("POST", `/v1/tenants/${tenantId}/servers/${serverId}/revoke`),
    onSuccess: (s) => {
      toast.success(`${s.label} revoked`, "Its credential no longer works. Add it again to re-enroll.");
      void qc.invalidateQueries({ queryKey: key(tenantId) });
    },
    onError: (e) => toast.error("Not revoked", e.message),
  });
}
