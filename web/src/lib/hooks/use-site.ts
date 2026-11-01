import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import { toast } from "../design/components/toast/toast-queue";

export type SiteRelease = { id: string; at: string };
export type SiteState = { enabled?: boolean; published: boolean; url?: string; current?: string; releases: SiteRelease[]; rrflow_recorded?: boolean; note?: string; scan?: string; dry_run?: boolean };
export type SiteAction = { action: "publish" | "unpublish" | "rollback"; slug: string; port: number; release?: string; path?: string; dry_run?: boolean };

/** Turn a workspace name into a site name (lowercase, dashes, max 32). */
export const siteSlug = (name: string) =>
  name.toLowerCase().replace(/[^a-z0-9-]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 32).replace(/-+$/, "") || "site";

export const useSite = (tenantId: string, id: string, slug: string) =>
  useQuery({
    queryKey: ["tenant", tenantId, "ws", id, "site", slug],
    queryFn: () => apiGet<SiteState>(`/v1/tenants/${tenantId}/workspaces/${id}/site?slug=${encodeURIComponent(slug)}`),
    enabled: !!slug,
  });

const done: Record<SiteAction["action"], string> = { publish: "Published", unpublish: "Unpublished", rollback: "Rolled back" };

export function useSiteAction(tenantId: string, id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (b: SiteAction) => apiSend<SiteState>("POST", `/v1/tenants/${tenantId}/workspaces/${id}/site`, b),
    onSuccess: (s, b) => toast.success(done[b.action], s.url ?? "The site is offline; releases are kept."),
    onError: (e, b) => toast.error(`Couldn't ${b.action}`, e.message),
    onSettled: () => void qc.invalidateQueries({ queryKey: ["tenant", tenantId, "ws", id, "site"] }),
  });
}
