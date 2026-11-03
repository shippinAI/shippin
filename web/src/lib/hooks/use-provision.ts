import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import { toast } from "../design/components/toast/toast-queue";

export type OperatorTenant = { id: string; idp_org_id: string; name: string; plan_id: string; status: string; ssh_ip?: string; error?: string; created_at: string };

export const useAllTenants = (enabled: boolean) =>
  useQuery({ queryKey: ["operator", "tenants"], queryFn: () => apiGet<{ items: OperatorTenant[] }>("/v1/tenants"), enabled, refetchInterval: 15_000 });

/** Re-run idempotent provisioning. The API returns a friendly reason to customers and the raw cause to operators. */
export function useRetryProvision() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id }: { id: string; name: string }) => apiSend<{ status: string }>("POST", `/v1/tenants/${id}/provision`),
    onMutate: ({ name }) => toast.info(`Setting up ${name}…`, "This can take up to a minute."),
    onSuccess: (_d, { name }) => toast.success(`${name} is ready`),
    onError: (e, { name }) => toast.error(`Setup for ${name} failed`, e.message),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ["me"] });
      void qc.invalidateQueries({ queryKey: ["operator", "tenants"] });
    },
  });
}
