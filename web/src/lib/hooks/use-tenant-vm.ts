import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import type { TenantVM, VMAction } from "../api/vm-types";
import { toast } from "../design/components/toast/toast-queue";

const key = (id: string) => ["admin", "tenant-vm", id];

/** Tenant VM state, sizes and nested containers (refreshes while the drawer is open). */
export function useTenantVM(id: string | undefined) {
  return useQuery({
    queryKey: key(id ?? ""),
    queryFn: () => apiGet<TenantVM>(`/v1/operator/tenants/${id}/vm`),
    enabled: !!id,
    refetchInterval: 10_000,
    retry: false,
  });
}

const verb: Record<VMAction | "enable", string> = { start: "Starting", stop: "Stopping", restart: "Restarting", enable: "Creating the tenant VM for" };

/** Enable (legacy → VM-backed) or start/stop/restart the tenant VM, with toasts. */
export function useTenantVMAction() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, action }: { id: string; name: string; action: VMAction | "enable" }) =>
      apiSend<unknown>("POST", `/v1/operator/tenants/${id}/vm${action === "enable" ? "" : `/${action}`}`),
    onMutate: ({ name, action }) => toast.info(`${verb[action]} ${name}…`),
    onSuccess: (_d, { name }) => toast.success(`${name}: tenant VM updated`),
    onError: (e, { name }) => toast.error(`Tenant VM action failed for ${name}`, e.message),
    onSettled: (_d, _e, { id }) => {
      void qc.invalidateQueries({ queryKey: key(id) });
      void qc.invalidateQueries({ queryKey: ["admin", "workspaces"] });
    },
  });
}
