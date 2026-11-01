// Platform access grants and the audit log (/v1/operator/access-requests, /v1/tenants/{id}/access).
export type GrantStatus = "pending" | "approved" | "denied" | "revoked" | "expired" | "ended" | "cancelled";

export type Grant = {
  id: string; tenant_id: string; tenant_name: string; workspace_id?: string; workspace_name?: string;
  kind: "request" | "breakglass"; requester: string; requester_name: string; reason: string; category?: string;
  duration_s: number; status: GrantStatus; decided_by?: string; decided_name?: string; created_at: string;
  decided_at?: string; expires_at?: string; ended_at?: string; notice_at?: string;
};

export type AuditEntry = {
  id: number; at: string; tenant_id?: string; tenant_name?: string; workspace_id?: string; workspace_name?: string;
  grant_id?: string; actor: string; actor_name: string; action: string; detail?: string; ip?: string; breakglass: boolean;
};

/** A fresh walk of the audit hash chain (GET /v1/operator/audit). */
export type AuditChain = { enabled: boolean; rows: number; ok: boolean; first_bad?: number; head?: string };

export type BreakGlassCategory = { id: string; label: string; withhold_notice?: boolean; notice_delay?: string };

export type AccessConfig = {
  durations: string[]; reason_min: number; reason_max: number;
  break_glass: { duration: string; reauth_minutes: number; justification_min: number; categories: BreakGlassCategory[] };
};

export type AccessRequestBody = {
  tenant_id: string; workspace_id?: string; reason?: string; duration?: string;
  category?: string; justification?: string; withhold_notice?: boolean;
};
