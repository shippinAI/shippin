import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiSend } from "../api/client";
import { toast } from "../design/components/toast/toast-queue";

type TenantAction = "provision" | "suspend" | "resume" | "delete";

const copy: Record<TenantAction, [string, string, string]> = {
  provision: ["Re-running setup for", "is provisioned", "Setup failed for"],
  suspend: ["Suspending", "is suspended", "Couldn't suspend"],
  resume: ["Resuming", "is active again", "Couldn't resume"],
  delete: ["Deleting", "was deleted", "Couldn't delete"],
};

/** Provider tenant actions with toasts; delete sends the typed-name confirmation. */
export function useTenantAction() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, action, confirm }: { id: string; name: string; action: TenantAction; confirm?: string }) =>
      action === "delete"
        ? apiSend<{ status: string }>("DELETE", `/v1/operator/tenants/${id}`, { confirm })
        : apiSend<{ status: string }>("POST", `/v1/operator/tenants/${id}/${action}`),
    onMutate: ({ name, action }) => toast.info(`${copy[action][0]} ${name}…`),
    onSuccess: (_d, { name, action }) => toast.success(`${name} ${copy[action][1]}`),
    onError: (e, { name, action }) => toast.error(`${copy[action][2]} ${name}`, e.message),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ["admin"] });
    },
  });
}

type WsAction = "start" | "stop" | "delete";
const wsCopy: Record<WsAction, string> = { start: "Starting", stop: "Stopping", delete: "Deleting" };

/** Start/stop/delete any workspace from the provider panel; delete sends the typed name. */
export function useAdminWorkspaceAction() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, action, confirm }: { id: string; name: string; action: WsAction; confirm?: string }) =>
      apiSend<{ state: string }>("POST", `/v1/operator/workspaces/${id}/${action}`, action === "delete" ? { confirm } : undefined),
    onSuccess: (_d, { name, action }) => toast.info(`${wsCopy[action]} ${name}…`),
    onError: (e, { name, action }) => toast.error(`Couldn't ${action} ${name}`, e.message),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ["admin", "workspaces"] });
    },
  });
}
