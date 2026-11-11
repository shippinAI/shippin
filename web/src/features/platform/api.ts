import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../../lib/api/client";
import { toast } from "../../lib/design/components/toast/toast-queue";

export type ServiceState = "up" | "down" | "disabled";
export type ServiceMode = "built-in" | "external" | "off";
export type BuiltInService = {
  id: string;
  name: string;
  mode: ServiceMode;
  state: ServiceState;
  endpoint?: string;
  version?: string;
  facts: { label: string; value: string }[];
  problem?: string;
  changeable: boolean;
};
export type SetupStep = { step: string; acknowledged_by: string; acknowledged_at: string };
export type Backup = { name: string; size_bytes: number; created_at: string };

const servicesKey = ["platform", "services"];
const stepsKey = ["platform", "steps"];
const backupsKey = ["platform", "backups"];

export const useBuiltInServices = () =>
  useQuery({ queryKey: servicesKey, queryFn: () => apiGet<{ items: BuiltInService[] }>("/v1/operator/built-in-services"), refetchInterval: 15_000 });

export const useSetupSteps = () => useQuery({ queryKey: stepsKey, queryFn: () => apiGet<{ items: SetupStep[] }>("/v1/operator/setup-steps") });

export const useBackups = () => useQuery({ queryKey: backupsKey, queryFn: () => apiGet<{ items: Backup[] }>("/v1/operator/backups") });

export function useAcknowledgeStep() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (step: string) => apiSend("PUT", `/v1/operator/setup-steps/${step}`, { reason: "Acknowledged during setup", understood: true }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: stepsKey }),
    onError: (e) => toast.error("Not saved", e.message),
  });
}

export function useSetClickHouse() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { mode: ServiceMode; url: string; reason: string }) => apiSend("PUT", "/v1/operator/built-in-services/clickhouse", { ...v, understood: true }),
    onSuccess: () => {
      toast.success("ClickHouse connection changed");
      return qc.invalidateQueries({ queryKey: servicesKey });
    },
    onError: (e) => toast.error("Not changed", e.message),
  });
}

export function useCreateBackup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (reason: string) => apiSend<{ backup: Backup }>("POST", "/v1/operator/backups", { reason }),
    onSuccess: (r) => {
      toast.success("Backup saved", r.backup.name);
      return qc.invalidateQueries({ queryKey: backupsKey });
    },
    onError: (e) => toast.error("Backup failed", e.message),
  });
}

export function useDeleteBackup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => apiSend("DELETE", `/v1/operator/backups/${name}?confirm_name=${encodeURIComponent(name)}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: backupsKey }),
    onError: (e) => toast.error("Not deleted", e.message),
  });
}

export const revealRecoveryKey = (reason: string) => apiSend<{ key: string }>("POST", "/v1/operator/recovery-key", { reason, understood: true });
