import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, apiGet, apiSend } from "../api/client";
import { toast } from "../design/components/toast/toast-queue";
import { useText } from "../ui-config/use-ui-config";

export type InfraField = { name: string; required: boolean; secret?: boolean; advanced?: boolean; label: string; help?: string; placeholder?: string; default?: string };
export type InfraGuestCount = { total: number; running: number };
export type ClusterMember = { name: string; online: boolean; local?: boolean };
export type NodeDetails = {
  hostname?: string;
  release?: string;
  os?: string;
  kernel?: string;
  cpu_model?: string;
  cpus?: number;
  mem_total_bytes?: number;
  mem_used_bytes?: number;
  uptime_seconds?: number;
  cluster?: string;
  members?: ClusterMember[];
};
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
  fleet_id?: string;
  details?: NodeDetails;
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
export type InfraFleet = { id: string; label: string; description?: string };
export type InfraOverview = {
  providers: InfraProvider[];
  totals: InfraTotals;
  fleets: InfraFleet[];
  unattached: { id: string; slug: string; label: string; site?: string }[];
  default_site?: string;
  local_agent?: { label: string; error: string };
  checked_at: string;
};
export type CertificateInfo = { fingerprint: string; subject: string; issuer: string; names: string[]; not_after: string; pem: string };
export type ConnectResult = { ok: boolean; version?: string; detail?: string; error?: string; nodes?: string[]; certificate?: CertificateInfo };
export type OperationStatus = "queued" | "running" | "succeeded" | "failed" | "unknown" | "skipped";
export type InfraOperation = {
  id: string;
  server_id: string;
  server_label?: string;
  batch_id?: string;
  guest: string;
  verb: string;
  status: OperationStatus;
  detail?: string;
  guest_status?: string;
  actor_name?: string;
  started_at: string;
  finished_at?: string;
};

const overviewKey = ["infra", "overview"];
const operationsKey = ["infra", "operations"];

export const useInfraOverview = (enabled = true) =>
  useQuery({ queryKey: overviewKey, queryFn: () => apiGet<InfraOverview>("/v1/infra/overview"), refetchInterval: 30_000, enabled });

const isOpen = (o: InfraOperation) => o.status === "queued" || o.status === "running";

/** Recent changes, for one host or all. Polls quickly only while something is in progress. */
export const useOperations = (serverId?: string) =>
  useQuery({
    queryKey: [...operationsKey, serverId ?? "all"],
    queryFn: () => apiGet<{ items: InfraOperation[] }>(`/v1/infra/operations?limit=30${serverId ? `&server=${encodeURIComponent(serverId)}` : ""}`),
    refetchInterval: (q) => (q.state.data?.items.some(isOpen) ? 2_000 : 20_000),
  });

export type HostValues = { kind: string; values: Record<string, string>; trustedCert?: string };
export type NewHost = HostValues & { label: string; slug: string; site: string };

const bodyOf = (v: HostValues) => ({ kind: v.kind, ...v.values, ...(v.trustedCert ? { trusted_cert: v.trustedCert } : {}) });

function useRefresh() {
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({ queryKey: ["infra"] });
  };
}

const boardWord = (fallback: string, e: Error) =>
  e instanceof ApiError && e.code === "waiting_on_board" ? (["Approve the card, then try again", e.message] as const) : ([fallback, e.message] as const);

/** Tries what the form holds. Stores nothing, and does not spend a one-time token. */
export const useTestHost = () => useMutation({ mutationFn: (v: HostValues) => apiSend<ConnectResult>("POST", "/v1/infra/servers/test", bodyOf(v)) });

/** Creates a host with its API, or attaches an API to a server the registry already has. */
export function useAddHost() {
  const refresh = useRefresh();
  const t = useText();
  return useMutation({
    mutationFn: (v: (NewHost & { existing?: undefined }) | (HostValues & { existing: string; label: string })) =>
      v.existing
        ? apiSend("PUT", `/v1/infra/servers/${v.existing}`, bodyOf(v))
        : apiSend("POST", "/v1/infra/servers", { label: v.label, slug: (v as NewHost).slug, site: (v as NewHost).site, ...bodyOf(v) }),
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

/** Stops the named guests in order. It answers at once; each guest's result arrives in the history. */
export function useStopGuests(serverId: string) {
  const refresh = useRefresh();
  const t = useText();
  return useMutation({
    mutationFn: (v: { guests: string[]; force: boolean }) => apiSend<{ batch_id: string; total: number }>("POST", `/v1/infra/servers/${serverId}/stop-guests`, v),
    onSuccess: (r) => {
      toast.info(t("infra.stop.started", { count: r.total }), t("infra.stop.startedBody"));
      refresh();
    },
    onError: (e) => toast.error(...boardWord(t("infra.stop.failed"), e)),
  });
}

export function useSaveFleet() {
  const refresh = useRefresh();
  const t = useText();
  return useMutation({
    mutationFn: (v: { id?: string; label: string; description: string }) =>
      v.id
        ? apiSend<InfraFleet>("PUT", `/v1/infra/fleets/${v.id}`, { label: v.label, description: v.description })
        : apiSend<InfraFleet>("POST", "/v1/infra/fleets", { label: v.label, description: v.description }),
    onSuccess: refresh,
    onError: (e) => toast.error(t("infra.fleets.saveFailed"), e.message),
  });
}

export function useDeleteFleet() {
  const refresh = useRefresh();
  const t = useText();
  return useMutation({
    mutationFn: (id: string) => apiSend("DELETE", `/v1/infra/fleets/${id}`),
    onSuccess: refresh,
    onError: (e) => toast.error(t("infra.fleets.deleteFailed"), e.message),
  });
}

export function useSetFleet() {
  const refresh = useRefresh();
  const t = useText();
  return useMutation({
    mutationFn: (v: { serverId: string; fleetId: string }) => apiSend("PUT", `/v1/infra/servers/${v.serverId}/fleet`, { fleet_id: v.fleetId }),
    onSuccess: refresh,
    onError: (e) => toast.error(t("infra.fleets.assignFailed"), e.message),
  });
}
