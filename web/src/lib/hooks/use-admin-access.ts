import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, apiGet, apiSend } from "../api/client";
import type { AccessConfig, AccessRequestBody, AuditChain, AuditEntry, Grant } from "../api/access-types";
import { toast } from "../design/components/toast/toast-queue";

export const useAdminGrants = () =>
  useQuery({
    queryKey: ["admin", "grants"],
    queryFn: () => apiGet<{ items: Grant[]; config: AccessConfig }>("/v1/operator/access-requests"),
    refetchInterval: 10_000,
    retry: false,
  });

export const useAdminAudit = (tenantId = "") =>
  useQuery({
    queryKey: ["admin", "audit", tenantId],
    queryFn: () => apiGet<{ items: AuditEntry[]; chain?: AuditChain }>(`/v1/operator/audit${tenantId ? `?tenant_id=${tenantId}` : ""}`),
    refetchInterval: 15_000,
    retry: false,
  });

/** Re-authenticate at the IdP and come back to this page (break-glass needs a fresh sign-in). */
export function reauthenticate() {
  const rd = window.location.pathname + window.location.search;
  window.location.assign(`/auth/login?reauth=1&rd=${encodeURIComponent(rd)}`);
}

/** Ask a tenant for access (kind "request") or activate break-glass (kind "breakglass"). */
export function useCreateGrant(kind: "request" | "breakglass") {
  const qc = useQueryClient();
  const path = kind === "request" ? "/v1/operator/access-requests" : "/v1/operator/break-glass";
  return useMutation({
    mutationFn: (body: AccessRequestBody) => apiSend<{ id: string; status: string; notice_at?: string }>("POST", path, body),
    onSuccess: (d) =>
      kind === "request"
        ? toast.success("Access requested", "The customer's owners and admins were asked to approve it.")
        : d.status === "pending"
          ? toast.info("Break-glass requested", "Another approver (Security or an Owner) must approve it. You can't approve your own.")
          : toast.error("Break-glass access is active", "Every action is audited. End it as soon as you're done."),
    onError: (e) => {
      if (e instanceof ApiError && e.status === 428) {
        toast.info("Sign in again", e.message);
        window.setTimeout(reauthenticate, 1200);
        return;
      }
      toast.error(kind === "request" ? "Couldn't request access" : "Break-glass not activated", e.message);
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ["admin"] });
      void qc.invalidateQueries({ queryKey: ["me"] });
    },
  });
}

/** Cancel your pending request or end your active grant early. */
export function useEndGrant() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (g: Grant) => apiSend<{ status: string }>("POST", `/v1/operator/access-requests/${g.id}/end`),
    onSuccess: (d, g) => toast.success(d.status === "cancelled" ? "Request cancelled" : "Access ended", g.tenant_name),
    onError: (e) => toast.error("Couldn't end access", e.message),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ["admin"] });
      void qc.invalidateQueries({ queryKey: ["me"] });
    },
  });
}

/** Break-glass requests waiting for an approver (approvers only; 403 for others). */
export const useBreakGlassPending = (enabled: boolean) =>
  useQuery({
    queryKey: ["admin", "bg-pending"],
    queryFn: () => apiGet<{ items: Grant[]; self: string; approval_minutes: number }>("/v1/operator/break-glass/pending"),
    refetchInterval: 10_000,
    retry: false,
    enabled,
  });

/** Approve or deny someone else's break-glass (fresh sign-in required). */
export function useDecideBreakGlass() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, decision }: { id: string; decision: "approve" | "deny" }) => apiSend<Grant>("POST", `/v1/operator/break-glass/${id}/${decision}`),
    onSuccess: (_d, v) => (v.decision === "approve" ? toast.error("Break-glass approved", "It is active for the requester now.") : toast.success("Break-glass denied")),
    onError: (e) => {
      if (e instanceof ApiError && e.status === 428) {
        toast.info("Sign in again", e.message);
        window.setTimeout(reauthenticate, 1200);
        return;
      }
      toast.error("Couldn't decide", e.message);
    },
    onSettled: () => void qc.invalidateQueries({ queryKey: ["admin"] }),
  });
}
