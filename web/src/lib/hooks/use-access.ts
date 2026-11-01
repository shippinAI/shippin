import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import type { AuditEntry, Grant } from "../api/access-types";
import { toast } from "../design/components/toast/toast-queue";

/** The tenant's access requests and grants (owners/admins only; others get 403). */
export const useTenantAccess = (tenantId?: string, enabled = true) =>
  useQuery({
    queryKey: ["tenant", tenantId, "access"],
    queryFn: () => apiGet<{ items: Grant[]; notices: Grant[] }>(`/v1/tenants/${tenantId}/access`),
    enabled: !!tenantId && enabled,
    refetchInterval: 15_000,
    retry: false,
  });

export const useTenantAudit = (tenantId?: string, enabled = true) =>
  useQuery({
    queryKey: ["tenant", tenantId, "audit"],
    queryFn: () => apiGet<{ items: AuditEntry[] }>(`/v1/tenants/${tenantId}/audit`),
    enabled: !!tenantId && enabled,
    refetchInterval: 30_000,
    retry: false,
  });

type Decision = "approve" | "deny" | "revoke";
const copy: Record<Decision, [string, string]> = {
  approve: ["Access approved", "Approval failed"],
  deny: ["Request denied", "Couldn't deny the request"],
  revoke: ["Access revoked", "Couldn't revoke access"],
};

/** Approve / deny a pending request or revoke an active grant, with toasts. */
export function useDecideAccess(tenantId?: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ grant, decision }: { grant: Grant; decision: Decision }) =>
      apiSend<Grant>("POST", `/v1/tenants/${tenantId}/access/${grant.id}/${decision}`),
    onSuccess: (_d, { grant, decision }) => toast.success(copy[decision][0], `${grant.requester_name} · ${grant.workspace_name || "whole project space"}`),
    onError: (e, { decision }) => toast.error(copy[decision][1], e.message),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ["tenant", tenantId] });
    },
  });
}
