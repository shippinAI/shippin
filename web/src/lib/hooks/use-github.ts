import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import type { GitHubRepos, ImportRequest } from "../api/github-types";
import { toast } from "../design/components/toast/toast-queue";

export const useGitHubRepos = (tenantId: string, enabled: boolean) =>
  useQuery({
    queryKey: ["tenant", tenantId, "github", "repos"],
    queryFn: () => apiGet<GitHubRepos>(`/v1/tenants/${tenantId}/github/repos`),
    enabled: enabled && !!tenantId,
    retry: false,
  });

/** Where an org owner/admin starts installing the Shippin GitHub App (server-side redirect). */
export const githubInstallURL = (tenantId: string) => `/v1/tenants/${tenantId}/github/install`;

export function useImportRepo(tenantId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: ImportRequest) =>
      apiSend<{ project_id: string; slug: string; repo: string; branch: string }>("POST", `/v1/tenants/${tenantId}/github/import`, body),
    onSuccess: (r) => {
      toast.success(`Project ${r.slug} imported`, `${r.repo}@${r.branch} is cloned into ~/code/${r.slug} in your devspace.`);
      void qc.invalidateQueries({ queryKey: ["tenant", tenantId, "projects"] });
    },
    onError: (e) => toast.error("Import failed", e.message),
  });
}

/** "Create GitHub repo & sync": repo_id 0 creates a private repo named after the project (org accounts). */
export function usePublishProject(tenantId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (b: { projectId: string; installation_id: number; repo_id?: number }) =>
      apiSend<{ repo: string }>("POST", `/v1/tenants/${tenantId}/projects/${b.projectId}/github/publish`, { installation_id: b.installation_id, repo_id: b.repo_id ?? 0 }),
    onSuccess: (r) => {
      toast.success(`Synced to ${r.repo}`, "The folder is pushed and linked.");
      void qc.invalidateQueries({ queryKey: ["tenant", tenantId] });
    },
  });
}
