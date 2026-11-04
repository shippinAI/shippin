import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import { toast } from "../design/components/toast/toast-queue";

export type PlatformSetting = {
  key: string;
  label: string;
  help: string;
  kind: "string" | "bool" | "integer" | "json" | "array";
  config_value: unknown;
  value: unknown;
  overridden: boolean;
};

const key = ["platform-settings"];
const agentHubKey = ["agenthub-connection"];

export const usePlatformSettings = () =>
  useQuery({ queryKey: key, queryFn: () => apiGet<{ items: PlatformSetting[] }>("/v1/operator/platform-settings") });

/** Saves (value) or clears (null = back to the config file) one override; audited server-side. */
export function useSetPlatformSetting() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { key: string; value: unknown; reason: string }) =>
      apiSend("PUT", `/v1/operator/platform-settings/${encodeURIComponent(v.key)}`, { value: v.value, reason: v.reason }),
    onSuccess: () => {
      toast.success("Setting saved");
      return qc.invalidateQueries({ queryKey: key });
    },
    onError: (e) => toast.error("Not saved", e.message),
  });
}

export type AgentHubConnection = { base_url: string; has_credential: boolean; configured: boolean };

export const useAgentHubConnection = () =>
  useQuery({ queryKey: agentHubKey, queryFn: () => apiGet<AgentHubConnection>("/v1/operator/agenthub-connection") });

export function useSetAgentHubConnection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { base_url: string; credential?: string; clear_credential?: boolean; reason: string }) =>
      apiSend("PUT", "/v1/operator/agenthub-connection", v),
    onSuccess: () => {
      toast.success("AgentHub connection settings saved");
      return qc.invalidateQueries({ queryKey: agentHubKey });
    },
    onError: (e) => toast.error("Not saved", e.message),
  });
}

export type AgentHubConnectionTest = { ok: boolean; error?: string; tenant_id?: string; policy_version?: number; policy_sha256?: string };

export function useTestAgentHubConnection() {
  return useMutation({
    mutationFn: (v: { tenant_id: string }) => apiSend<AgentHubConnectionTest>("POST", "/v1/operator/agenthub-connection/test", v),
    onError: (e) => toast.error("Connection test failed", e.message),
  });
}
