import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import { toast } from "../design/components/toast/toast-queue";

export type PlatformServiceObservation = {
  id: string;
  service_id: string;
  config_revision: number;
  observed_at: string;
  configured: boolean;
  healthy: boolean;
  latency_ms?: number;
  detail: string;
  resource?: { observed: boolean; status?: string; detail: string };
};

export type PlatformService = {
  id: string;
  name: string;
  kind: string;
  alias?: string;
  url?: string;
  host?: string;
  tcp?: string;
  instance?: string;
  server_id?: string;
  guest_id?: string;
  timeout_seconds: number;
  interval_seconds: number;
  enabled: boolean;
  revision: number;
  state: "unknown" | "stale" | "operational" | "down" | "disabled";
  observation?: PlatformServiceObservation;
};

export type PlatformServiceInput = Omit<PlatformService, "id" | "enabled" | "state" | "observation" | "revision"> & { reason: string };
const key = ["platform-health-services"];

export function usePlatformServices(enabled = true) {
  return useQuery({ queryKey: key, enabled, queryFn: () => apiGet<{ items: PlatformService[] }>("/v1/operator/platform-services") });
}

export function usePlatformServiceObservations(serviceId: string, enabled: boolean) {
  return useQuery({
    queryKey: [...key, serviceId, "observations"],
    enabled: enabled && Boolean(serviceId),
    queryFn: () => apiGet<{ items: PlatformServiceObservation[] }>(`/v1/operator/platform-services/${serviceId}/observations?limit=20`),
  });
}

export function usePlatformServiceMutation() {
  const qc = useQueryClient();
  const refresh = () => qc.invalidateQueries({ queryKey: key });
  return {
    save: useMutation({
      mutationFn: (input: PlatformServiceInput & { id?: string; revision?: number }) => {
        const { id, ...body } = input;
        return apiSend(id ? "PUT" : "POST", id ? `/v1/operator/platform-services/${id}` : "/v1/operator/platform-services", body);
      },
      onSuccess: () => { toast.success("Platform service saved"); return refresh(); },
      onError: (e) => toast.error("Service not saved", e.message),
    }),
    disable: useMutation({
      mutationFn: (service: PlatformService & { reason: string }) => apiSend("DELETE", `/v1/operator/platform-services/${service.id}`, { revision: service.revision, reason: service.reason }),
      onSuccess: () => { toast.success("Platform service disabled"); return refresh(); },
      onError: (e) => toast.error("Service not disabled", e.message),
    }),
    check: useMutation({
      mutationFn: (id: string) => apiSend("POST", `/v1/operator/platform-services/${id}/check`, {}),
      onSuccess: () => { toast.success("Health check recorded"); return refresh(); },
      onError: (e) => toast.error("Health check failed", e.message),
    }),
  };
}
