import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import type { Workspace, WorkspaceList } from "../api/types";
import { toast } from "../design/components/toast/toast-queue";

export const busy = (w: Workspace) => ["provisioning", "starting", "stopping", "deleting"].includes(w.state);

/** Workspaces of a tenant; polls fast while anything is in flight. */
export const useWorkspaces = (tenantId?: string) =>
  useQuery({
    queryKey: ["tenant", tenantId, "workspaces"],
    queryFn: () => apiGet<WorkspaceList>(`/v1/tenants/${tenantId}/workspaces`),
    enabled: !!tenantId,
    refetchInterval: (q) => (q.state.data?.items.some(busy) ? 3000 : 20_000),
  });

export function useCreateWorkspace(tenantId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { name: string; template_id?: string; repo?: string; size?: string }) =>
      apiSend<Workspace>("POST", `/v1/tenants/${tenantId}/workspaces`, body),
    onSuccess: (w) => {
      toast.info(`Creating ${w.name}…`, "Setting up the server, storage, HTTPS address and your SSH keys.");
      void qc.invalidateQueries({ queryKey: ["tenant", tenantId, "workspaces"] });
    },
    onError: (e) => toast.error("Couldn't create workspace", e.message),
  });
}

export type WsAction = "start" | "stop" | "restart" | "delete";
const verbs: Record<WsAction, [string, string]> = {
  start: ["Starting", "is starting"], stop: ["Stopping", "is stopping"],
  restart: ["Restarting", "is restarting"], delete: ["Deleting", "is being deleted"],
};

export function useWorkspaceAction(tenantId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, action }: { id: string; name: string; action: WsAction }) =>
      apiSend("POST", `/v1/tenants/${tenantId}/workspaces/${id}/actions/${action}`),
    onSuccess: (_d, { name, action }) => {
      toast.info(`${name} ${verbs[action][1]}`, "We'll let you know when it's done.");
      void qc.invalidateQueries({ queryKey: ["tenant", tenantId, "workspaces"] });
    },
    onError: (e, { name, action }) => toast.error(`${verbs[action][0]} ${name} failed`, e.message),
  });
}

/** PUT firewall rules or custom domains for a workspace. */
export function useWorkspaceAccess(tenantId: string, id: string, field: "firewall" | "domains") {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: object) => apiSend("PUT", `/v1/tenants/${tenantId}/workspaces/${id}/${field}`, body),
    onSuccess: () => {
      toast.success(field === "firewall" ? "Firewall updated" : "Domains updated");
      void qc.invalidateQueries({ queryKey: ["tenant", tenantId, "workspaces"] });
    },
    onError: (e) => toast.error("Change not applied", e.message),
  });
}
