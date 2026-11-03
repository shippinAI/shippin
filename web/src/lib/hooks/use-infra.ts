import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, apiGet, apiSend } from "../api/client";
import { toast } from "../design/components/toast/toast-queue";
import { useText } from "../ui-config/use-ui-config";

export type InfraField = { name: string; required: boolean; label: string; help?: string; placeholder?: string; default?: string };
export type InfraGuestCount = { total: number; running: number };
export type InfraNode = {
  id: string;
  slug?: string;
  label: string;
  site?: string;
  kind: string;
  /** "agent" is the local agent's read-only view; "api" is an attached hypervisor API. */
  source: "api" | "agent";
  base_url?: string;
  scope?: string;
  online: boolean;
  version?: string;
  error?: string;
  guests: InfraGuestCount;
};
export type InfraTotals = { nodes: number; online: number; guests: number; running: number };
export type InfraProvider = {
  kind: string;
  label: string;
  description?: string;
  attachable: boolean;
  capabilities: string[];
  verbs: string[];
  fields: InfraField[];
  nodes: InfraNode[];
  totals: InfraTotals;
};
export type InfraOverview = {
  providers: InfraProvider[];
  totals: InfraTotals;
  unattached: { id: string; slug: string; label: string; site?: string }[];
  default_site?: string;
  local_agent?: { label: string; error: string };
  checked_at: string;
};

const overviewKey = ["infra", "overview"];

export const useInfraOverview = (enabled = true) =>
  useQuery({ queryKey: overviewKey, queryFn: () => apiGet<InfraOverview>("/v1/infra/overview"), refetchInterval: 30_000, enabled });

export type HostValues = { kind: string; values: Record<string, string> };
export type NewHost = HostValues & { label: string; slug: string; site: string };

function useRefresh() {
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({ queryKey: ["infra"] });
  };
}

const boardWord = (fallback: string, e: Error) =>
  e instanceof ApiError && e.code === "waiting_on_board" ? (["Approve the card, then try again", e.message] as const) : ([fallback, e.message] as const);

/** Creates a host with its API, or attaches an API to a server the registry already has. */
export function useAddHost() {
  const refresh = useRefresh();
  const t = useText();
  return useMutation({
    mutationFn: (v: (NewHost & { existing?: undefined }) | (HostValues & { existing: string; label: string })) =>
      v.existing
        ? apiSend("PUT", `/v1/infra/servers/${v.existing}`, { kind: v.kind, ...v.values })
        : apiSend("POST", "/v1/infra/servers", { kind: v.kind, label: v.label, slug: (v as NewHost).slug, site: (v as NewHost).site, ...v.values }),
    onSuccess: (_d, v) => {
      toast.success(t("infra.toast.added", { label: v.label }), t("infra.toast.addedBody"));
      refresh();
    },
    onError: (e) => toast.error(...boardWord(t("infra.toast.addFailed"), e)),
  });
}

export function useDetachHost() {
  const refresh = useRefresh();
  const t = useText();
  return useMutation({
    mutationFn: (v: { id: string; label: string }) => apiSend("DELETE", `/v1/infra/servers/${v.id}/attachment`),
    onSuccess: (_d, v) => {
      toast.info(t("infra.toast.detached", { label: v.label }), t("infra.toast.detachedBody"));
      refresh();
    },
    onError: (e) => toast.error(...boardWord(t("infra.toast.detachFailed"), e)),
  });
}
