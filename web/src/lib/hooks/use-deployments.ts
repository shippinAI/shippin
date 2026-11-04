import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import type { Json } from "../api/catalog-types";
import { toast } from "../design/components/toast/toast-queue";

export type DeploymentSource = { kind: "github" | "git" | "oci"; reference: string; commit?: string; context_path?: string };
export type DeploymentService = {
  id: string; label: string; description: string; kind: string; docs_url: string;
  config_schema: { type?: string; title?: string; description?: string; properties?: Record<string, JsonSchemaProperty>; required?: string[] };
  default_config: Record<string, Json>; deployment_source: DeploymentSource;
};
export type JsonSchemaProperty = {
  type?: string; title?: string; description?: string; format?: string; enum?: Json[];
  minimum?: number; maximum?: number; properties?: Record<string, JsonSchemaProperty>; items?: JsonSchemaProperty;
};
export type DeploymentTarget = { id: string; label: string; site: string; provider: "incus" | "proxmox"; state: "planned" | "pending" | "active" | "stale" | "revoked" };
export type DeploymentTargets = { items: DeploymentTarget[]; can_request: boolean };
export type Deployment = {
  id: string; service_id: string; target_system_id: string; target_provider: "incus" | "proxmox";
  generation: number; desired_state: "present" | "absent";
  observed_state: "unknown" | "pending" | "provisioning" | "ready" | "degraded" | "failed" | "absent";
  definition_sha256: string; config_schema: DeploymentService["config_schema"];
  source: DeploymentSource; config: Record<string, Json>;
  created_by: string; created_at: string; updated_by: string; updated_at: string;
};
export type DeploymentEvent = {
  id: string; deployment_id: string; tenant_id: string; generation: number; event_type: string;
  observed_state?: string; provider_operation_id?: string; health_status?: string; health_duration_ms?: number;
  failure_code?: string; actor: string; recorded_at: string;
};
type Items<T> = { items: T[]; page?: { next_page_token: string | null; total_estimate: number } };

const collection = (tenantId: string) => `/v1/tenants/${encodeURIComponent(tenantId)}/deployments`;
const key = (tenantId: string) => ["deployments", tenantId];

export const useDeployableServices = (tenantId: string) => useQuery({
  queryKey: ["deployable-services", tenantId],
  queryFn: () => apiGet<Items<DeploymentService>>(`/v1/tenants/${encodeURIComponent(tenantId)}/deployable-services`),
  enabled: !!tenantId,
  retry: false,
});

export const useDeploymentTargets = (tenantId: string) => useQuery({
  queryKey: ["deployment-targets", tenantId],
  queryFn: () => apiGet<DeploymentTargets>(`/v1/tenants/${encodeURIComponent(tenantId)}/deployment-targets`),
  enabled: !!tenantId,
  retry: false,
});

export const useDeployments = (tenantId: string) => useQuery({
  queryKey: key(tenantId),
  queryFn: () => apiGet<Items<Deployment>>(collection(tenantId)),
  enabled: !!tenantId,
  retry: false,
});

export const useDeploymentEvents = (tenantId: string, deploymentId: string) => useQuery({
  queryKey: [...key(tenantId), deploymentId, "events"],
  queryFn: () => apiGet<Items<DeploymentEvent>>(`${collection(tenantId)}/${encodeURIComponent(deploymentId)}/events`),
  enabled: !!tenantId && !!deploymentId,
  retry: false,
});

function useDeploymentMutation<TInput>(tenantId: string, mutationFn: (input: TInput) => Promise<Deployment>, success: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn,
    onSuccess: () => toast.success(success, "Intent was recorded. Provider state changes only after configured review and reconciliation."),
    onError: (error) => toast.error("Deployment request not saved", error.message),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: key(tenantId) });
      void qc.invalidateQueries({ queryKey: ["deployment-targets", tenantId] });
    },
  });
}

export function useCreateDeployment(tenantId: string) {
  return useDeploymentMutation(tenantId, (input: { service_id: string; target_system_id: string; desired_state: "present"; config: Record<string, Json> }) =>
    apiSend<Deployment>("POST", collection(tenantId), input), "Deployment request recorded");
}

export function useUpdateDeployment(tenantId: string) {
  return useDeploymentMutation(tenantId, (input: { deployment: Deployment; config: Record<string, Json> }) =>
    apiSend<Deployment>("PATCH", `${collection(tenantId)}/${encodeURIComponent(input.deployment.id)}`,
      { desired_state: input.deployment.desired_state, config: input.config }, { ifMatch: `"${input.deployment.generation}"` }), "Deployment update recorded");
}

export function useRequestDeploymentRemoval(tenantId: string) {
  return useDeploymentMutation(tenantId, (deployment: Deployment) =>
    apiSend<Deployment>("DELETE", `${collection(tenantId)}/${encodeURIComponent(deployment.id)}`, undefined,
      { ifMatch: `"${deployment.generation}"` }), "Removal request recorded");
}
