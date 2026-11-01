import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import { toast } from "../design/components/toast/toast-queue";

/** A devspace folder that looks like a project (git repo or package manifest). */
export type FoundFolder = {
  name: string;
  path: string;
  git: boolean;
  remote?: string;
  manifests: string[];
  slug: string;
  adopted: boolean;
  project_id?: string;
};

type AdoptResult = { slug: string; rrflow: string; project: { id: string; public_id: string }; warnings?: string[] };

const key = (t?: string, w?: string) => ["tenant", t, "workspaces", w, "discover"];

export const useDiscover = (tenantId?: string, workspaceId?: string) =>
  useQuery({
    queryKey: key(tenantId, workspaceId),
    queryFn: () => apiGet<{ items: FoundFolder[]; note?: string }>(`/v1/tenants/${tenantId}/workspaces/${workspaceId}/discover`),
    enabled: !!tenantId && !!workspaceId,
  });

export function useAdopt(tenantId: string, workspaceId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (path: string) => apiSend<AdoptResult>("POST", `/v1/tenants/${tenantId}/workspaces/${workspaceId}/adopt`, { path }),
    onSuccess: (r) => {
      toast.success(`Project ${r.slug} added`, r.warnings?.[0] ?? "It's yours, with its own rrflow space.");
      void qc.invalidateQueries({ queryKey: ["tenant", tenantId] });
    },
    onError: (e) => toast.error("Project not added", e.message),
  });
}
