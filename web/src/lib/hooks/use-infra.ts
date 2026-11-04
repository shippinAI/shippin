import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, apiGet, apiSend, isWaitingOnBoard } from "../api/client";
import { toast } from "../design/components/toast/toast-queue";
import { useText } from "../ui-config/use-ui-config";
import { useMe } from "./use-api";

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
  /** This host runs the panel itself. */
  control_plane?: boolean;
};
export type SystemIdentity = {
  server_id: string;
  label: string;
  hostname?: string;
  hostname_source: string;
  hostname_observed_at?: string;
  aliases: string[];
  tags: string[];
  agenthub_tenant_id?: string;
  agenthub_machine_id?: string;
  identity_updated_by?: string;
  identity_updated_at?: string;
};
export type InfraTotals = { nodes: number; online: number; guests: number; running: number };
export type InfraProvider = {
  kind: string;
  label: string;
  description?: string;
  attachable: boolean;
  capabilities: string[];
  verbs: string[];
  create_fields?: string[];
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
  note?: string;
  status: OperationStatus;
  detail?: string;
  guest_status?: string;
  actor_name?: string;
  started_at: string;
  finished_at?: string;
};

export type NetworkProvider = "cloudflare_tunnel" | "tailscale" | "custom";
export type NetworkConnection = {
  id: string; label: string; provider: NetworkProvider; config: Record<string, string>;
  has_credential: boolean; status: "unknown" | "healthy" | "degraded"; last_checked?: string; last_error?: string;
  sync_interval_seconds: number; created_at: string; updated_at: string;
};
export type NetworkResource = {
  id: string; connection_id: string; provider: NetworkProvider; kind: string; name: string;
  state?: string; address?: string; details?: Record<string, string>; observed_at: string;
};
export type NetworkConnectionInput = { label: string; provider: NetworkProvider; config: Record<string, string>; credential?: string; sync_interval_seconds?: number };
export type NetworkTestResult = { ok: boolean; error?: string; resource_count?: number; resources?: NetworkResource[] };

const overviewKey = ["infra", "overview"];
const operationsKey = ["infra", "operations"];
const networkKey = ["infra", "networks"];
const systemIdentityKey = (id: string) => ["infra", "system-identity", id];

export const useSystemIdentity = (id: string) => useQuery({
  queryKey: systemIdentityKey(id),
  queryFn: () => apiGet<SystemIdentity>(`/v1/infra/servers/${encodeURIComponent(id)}/identity`),
  enabled: !!id,
});

export function useSaveSystemIdentity(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { label: string; aliases: string[]; tags: string[]; agenthub_tenant_id: string; agenthub_machine_id: string; note: string }) =>
      apiSend<SystemIdentity>("PUT", `/v1/infra/servers/${encodeURIComponent(id)}/identity`, {
        label: v.label, aliases: v.aliases, tags: v.tags, agenthub_tenant_id: v.agenthub_tenant_id, agenthub_machine_id: v.agenthub_machine_id,
      }, { note: v.note }),
    onSuccess: async (v) => {
      qc.setQueryData(systemIdentityKey(id), v);
      await qc.invalidateQueries({ queryKey: ["infra", "overview"] });
      toast.success("System identity saved", "The AgentHub reference is stored but not remotely verified.");
    },
    onError: (e) => toast.error("Could not save system identity", e.message),
  });
}

export const useInfraOverview = (enabled = true) =>
  useQuery({ queryKey: overviewKey, queryFn: () => apiGet<InfraOverview>("/v1/infra/overview"), refetchInterval: 30_000, enabled });

export const useNetworkConnections = () => useQuery({
  queryKey: networkKey,
  queryFn: () => apiGet<{ items: NetworkConnection[] }>("/v1/infra/networks"),
  refetchInterval: 30_000,
});

export const useNetworkResources = (id: string, enabled = true) => useQuery({
  queryKey: [...networkKey, id, "resources"],
  queryFn: () => apiGet<{ items: NetworkResource[] }>(`/v1/infra/networks/${encodeURIComponent(id)}/resources`),
  enabled,
});

export const useTestNetwork = () => useMutation({
  mutationFn: (v: NetworkConnectionInput) => apiSend<NetworkTestResult>("POST", "/v1/infra/networks/test", v),
});

export function useSaveNetwork() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: NetworkConnectionInput & { id?: string; note: string }) => apiSend<NetworkConnection>(
      v.id ? "PUT" : "POST", v.id ? `/v1/infra/networks/${encodeURIComponent(v.id)}` : "/v1/infra/networks",
      { label: v.label, provider: v.provider, config: v.config, sync_interval_seconds: v.sync_interval_seconds, ...(v.credential ? { credential: v.credential } : {}) }, { note: v.note },
    ),
    onSuccess: async () => { await qc.invalidateQueries({ queryKey: networkKey }); toast.success("Network integration saved", "Provider configuration is stored securely."); },
    onError: (e) => toast.error("Could not save network integration", e.message),
  });
}

export function useSyncNetwork() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiSend<{ items: NetworkResource[]; checked_at: string }>("POST", `/v1/infra/networks/${encodeURIComponent(id)}/sync`),
    onSuccess: async (_v, id) => { await qc.invalidateQueries({ queryKey: networkKey }); await qc.invalidateQueries({ queryKey: [...networkKey, id, "resources"] }); },
  });
}

export function useDeleteNetwork() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { id: string; note: string }) => apiSend("DELETE", `/v1/infra/networks/${encodeURIComponent(v.id)}`, undefined, { note: v.note }),
    onSuccess: async () => { await qc.invalidateQueries({ queryKey: networkKey }); toast.info("Network integration removed", "Its observed inventory was removed from this installation."); },
    onError: (e) => toast.error("Could not remove network integration", e.message),
  });
}

export const isOpen = (o: InfraOperation) => o.status === "queued" || o.status === "running";

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

/** can(p) for one infrastructure permission. The API decides the same thing again; this only hides what would be refused. */
export function useInfraCan() {
  const caps = useMe().data?.capabilities;
  return (p: string) => caps?.[p] === true;
}

/** Approves one waiting change. It runs when the page asks for it again. */
export const useApproveCard = () =>
  useMutation({ mutationFn: (id: string) => apiSend("POST", `/v1/hub/tasks/${encodeURIComponent(id)}/decision`, { approve: true, note: "" }) });

/** Tries what the form holds. Stores nothing, and does not spend a one-time token. */
export const useTestHost = () => useMutation({ mutationFn: (v: HostValues) => apiSend<ConnectResult>("POST", "/v1/infra/servers/test", bodyOf(v)) });

/** Creates a host with its API, or attaches an API to a server the registry already has. */
export function useAddHost() {
  const refresh = useRefresh();
  const t = useText();
  return useMutation({
    mutationFn: (v: ((NewHost & { existing?: undefined }) | (HostValues & { existing: string; label: string })) & { note: string }) =>
      v.existing
        ? apiSend("PUT", `/v1/infra/servers/${v.existing}`, bodyOf(v), { note: v.note })
        : apiSend("POST", "/v1/infra/servers", { label: v.label, slug: (v as NewHost).slug, site: (v as NewHost).site, ...bodyOf(v) }, { note: v.note }),
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
    mutationFn: (v: { id: string; label: string; note: string }) => apiSend("DELETE", `/v1/infra/servers/${v.id}/attachment`, undefined, { note: v.note }),
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
    mutationFn: ({ note, ...body }: { guests: string[]; force: boolean; note: string }) => apiSend<{ batch_id: string; total: number }>("POST", `/v1/infra/servers/${serverId}/stop-guests`, body, { note }),
    onSuccess: (r) => {
      toast.info(t("infra.stop.started", { count: r.total }), t("infra.stop.startedBody"));
      refresh();
    },
    onError: (e) => {
      if (!isWaitingOnBoard(e)) toast.error(t("infra.stop.failed"), e.message);
    },
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
