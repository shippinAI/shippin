import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import { toast } from "../design/components/toast/toast-queue";

export type PlatformSetting = {
  key: string;
  label: string;
  help: string;
  kind: "string" | "bool" | "json";
  config_value: unknown;
  value: unknown;
  overridden: boolean;
};

const key = ["platform-settings"];

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
