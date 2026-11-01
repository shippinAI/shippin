import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import type { Workspace, WorkspaceList } from "../api/types";
import { toast } from "../design/components/toast/toast-queue";

/** The devspace of a project environment: its first workspace there (M1: one per env). */
export const devspaceOf = (items: Workspace[] | undefined, projectId?: string, envId?: string) =>
  items?.find((w) => w.project_id === projectId && (!envId || w.environment_id === envId));

/**
 * One click: reuse the project's devspace (start it if stopped, keeping its home volume and
 * IDE state) or create it. The workspace name is derived from project + env, never typed.
 */
export function useOpenDevspace(tenantId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ project, env }: { project: { id: string; name: string }; env: { id: string; name: string } }) => {
      const list = await apiGet<WorkspaceList>(`/v1/tenants/${tenantId}/workspaces`);
      const ws = devspaceOf(list.items, project.id, env.id);
      if (ws && ws.state === "stopped") await apiSend("POST", `/v1/tenants/${tenantId}/workspaces/${ws.id}/actions/start`);
      if (ws) return ws;
      return apiSend<Workspace>("POST", `/v1/tenants/${tenantId}/workspaces`, {
        name: `${project.name}-${env.name}`.slice(0, 40), project_id: project.id, environment: env.name,
      });
    },
    onSettled: () => void qc.invalidateQueries({ queryKey: ["tenant", tenantId, "workspaces"] }),
    onError: (e) => toast.error("Couldn't open the devspace", e.message),
  });
}
