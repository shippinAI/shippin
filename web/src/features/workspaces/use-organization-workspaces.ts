import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../../lib/api/client";

export type OrganizationWorkspace = {
  id: string;
  name: string;
  description: string;
  is_default: boolean;
  projects: number;
  created_at: string;
  updated_at: string;
};
export type WorkspaceInput = Pick<OrganizationWorkspace, "name" | "description">;
const base = (tenant: string) => `/v1/tenants/${tenant}/organization-workspaces`;
export const workspaceKey = (tenant: string) => ["tenant", tenant, "organization-workspaces"];

export function useOrganizationWorkspaces(tenant: string) {
  return useQuery({ queryKey: workspaceKey(tenant), queryFn: () => apiGet<OrganizationWorkspace[]>(base(tenant)), enabled: !!tenant });
}

export function useSaveOrganizationWorkspace(tenant: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...input }: WorkspaceInput & { id?: string }) =>
      apiSend<OrganizationWorkspace>(id ? "PATCH" : "POST", `${base(tenant)}${id ? `/${id}` : ""}`, input),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: workspaceKey(tenant) }); },
  });
}

export function useDeleteOrganizationWorkspace(tenant: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiSend("DELETE", `${base(tenant)}/${id}`),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: workspaceKey(tenant) }); },
  });
}
