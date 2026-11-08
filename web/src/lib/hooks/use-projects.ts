import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import type { Project } from "../api/project-types";
import { toast } from "../design/components/toast/toast-queue";

const key = (t?: string) => ["tenant", t, "projects"];

export const useProjects = (tenantId?: string, workspaceId?: string) =>
  useQuery({
    queryKey: [...key(tenantId), "list", workspaceId ?? "all"],
    queryFn: () => apiGet<Project[]>(workspaceId ? `/v1/tenants/${tenantId}/organization-workspaces/${workspaceId}/projects` : `/v1/tenants/${tenantId}/projects`),
    enabled: !!tenantId,
  });

export const useProject = (tenantId?: string, projectId?: string) =>
  useQuery({
    queryKey: [...key(tenantId), projectId],
    queryFn: () => apiGet<Project>(`/v1/tenants/${tenantId}/projects/${projectId}`),
    enabled: !!tenantId && !!projectId,
  });

export function useCreateProject(tenantId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { name: string; environment: string; workspace_id?: string }) =>
      apiSend<Project>("POST", `/v1/tenants/${tenantId}/projects`, body),
    onSuccess: (p) => {
      toast.success(`Project ${p.name} created`, p.placed ? "It has a home and is ready for a devspace." : "It will be ready once your private server is up.");
      void qc.invalidateQueries({ queryKey: key(tenantId) });
      void qc.invalidateQueries({ queryKey: ["tenant", tenantId, "organization-workspaces"] });
    },
    onError: (e) => toast.error("Project not created", e.message),
  });
}

/** Last project the user had open, so the console can offer "resume" (per browser). */
export const lastProject = {
  get: (t: string) => localStorage.getItem(`shippin.last-project.${t}`) ?? undefined,
  set: (t: string, p: string) => localStorage.setItem(`shippin.last-project.${t}`, p),
};
