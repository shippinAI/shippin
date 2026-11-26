import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { callAction } from "../api/actions";
import { apiGet, apiSend } from "../api/client";
import { toast } from "../design/components/toast/toast-queue";

export type GuestMetrics = {
  cpu_fraction: number;
  cpu_count?: number;
  memory_used_bytes: number;
  memory_total_bytes: number;
  disk_used_bytes: number;
  disk_total_bytes: number;
  network_in_bytes: number;
  network_out_bytes: number;
};
export type Guest = {
  id: string;
  name: string;
  kind: string;
  status: string;
  node: string;
  tags?: string;
  pool?: string;
  template?: boolean;
  uptime_seconds?: number;
  metrics?: GuestMetrics | null;
  self?: boolean;
};

export type Task = { upid: string; status: string; exitstatus: string; type: string; log?: string[]; done: boolean };

const STAFF = "/v1/infra/servers/";

/** The server a staff path names. It is "" for an organization's own servers, which keep their own routes. */
const staffServer = (base: string) => (base.startsWith(STAFF) ? decodeURIComponent(base.slice(STAFF.length)) : "");

export type PowerVerb = "start" | "stop" | "shutdown" | "reboot";
const powerAction = {
  start: "shippin.infrastructure.server.guest.start",
  stop: "shippin.infrastructure.server.guest.stop",
  shutdown: "shippin.infrastructure.server.guest.shutdown",
  reboot: "shippin.infrastructure.server.guest.restart",
} as const;

/** Runs a power change on a guest: the staff action on a server staff manage, the organization's route otherwise. */
export function sendGuestPower(base: string, guest: string, verb: PowerVerb, o: { note: string; confirmSelf?: boolean; review?: string }) {
  const serverId = staffServer(base);
  if (serverId) {
    return callAction(
      powerAction[verb],
      { serverId, guestId: guest },
      { reason: o.note, confirm_self: o.confirmSelf || undefined, review: o.review },
    );
  }
  return apiSend(
    "POST",
    `${base}/guests/${encodeURIComponent(guest)}/status/${verb}${o.confirmSelf ? "?confirm=self" : ""}`,
    {},
    { note: o.note, ...(o.review ? { headers: { "X-AgentHub-Review-ID": o.review } } : {}) },
  );
}

/** One live inventory cache for every screen using this attached host. */
export const guestInventoryQuery = (base: string) => ({
  queryKey: ["hv", base, "guests"],
  queryFn: (): Promise<{ items: Guest[] }> =>
    staffServer(base)
      ? callAction("shippin.infrastructure.server.guest.list", { serverId: staffServer(base) })
      : apiGet<{ items: Guest[] }>(`${base}/guests`),
  enabled: !!base,
  refetchInterval: 15_000,
});

export const useGuests = (base: string) => useQuery(guestInventoryQuery(base));

export const useSnapshots = (base: string, guest: string) =>
  useQuery({
    queryKey: ["hv", base, guest, "snapshots"],
    queryFn: () => apiGet<{ items: string[] }>(`${base}/guests/${guest}/snapshots`),
    enabled: !!base && !!guest,
  });

export const useTask = (base: string, upid: string) =>
  useQuery({
    queryKey: ["hv", base, "task", upid],
    queryFn: () => apiGet<Task>(`${base}/tasks/${encodeURIComponent(upid)}`),
    enabled: !!base && !!upid,
    refetchInterval: (q) => (q.state.data?.done ? false : 2000),
  });

function reportError(fallback: string, e: Error) {
  toast.error(fallback, e.message);
}

/** Refresh operations after a mutation request has had time to be accepted. */
const showStarted = (qc: ReturnType<typeof useQueryClient>) => {
  setTimeout(() => void qc.invalidateQueries({ queryKey: ["infra", "operations"] }), 500);
};

export type NewGuest = {
  kind: string;
  name: string;
  image: string;
  storage?: string;
  network?: string;
  memory: number;
  cores: number;
  disk: number;
  start: boolean;
};
export type GuestImage = { id: string; label: string; kind?: string };
export type GuestStorage = { name: string; type: string; content?: string; node?: string };
export type StorageVolume = GuestStorage & { used_bytes: number; total_bytes: number };
export type GuestNetwork = { name: string; type: string; node?: string };

/** What a guest can be made from on this host. */
export const useGuestImages = (base: string) =>
  useQuery({ queryKey: ["hv", base, "images"], queryFn: () => apiGet<{ items: GuestImage[] }>(`${base}/images`), enabled: base !== "" });

/** Placement choices are read from this attached provider, never supplied as panel defaults. */
export const useGuestStorage = (base: string) =>
  useQuery({
    queryKey: ["hv", base, "storage"],
    queryFn: () => apiGet<{ items: GuestStorage[] }>(`${base}/storage`),
    enabled: base !== "",
  });

/** Live provider storage inventory and capacity; unsupported drivers do not issue this request. */
export const useStorageInventory = (base: string, enabled: boolean) =>
  useQuery({
    queryKey: ["hv", base, "storage-inventory"],
    queryFn: () => apiGet<{ items: StorageVolume[] }>(`${base}/storage`),
    enabled: !!base && enabled,
    refetchInterval: 30_000,
  });

export const useGuestNetworks = (base: string) =>
  useQuery({
    queryKey: ["hv", base, "networks"],
    queryFn: () => apiGet<{ items: GuestNetwork[] }>(`${base}/network`),
    enabled: base !== "",
  });

/** Removes a guest, stopping it first if it has to. */
export function useDeleteGuest(base: string) {
  const qc = useQueryClient();
  return useMutation({
    onMutate: () => showStarted(qc),
    mutationFn: (v: { guest: string; confirmSelf?: boolean; note?: string }) =>
      callAction(
        "shippin.infrastructure.server.guest.delete",
        { serverId: staffServer(base), guestId: v.guest },
        { confirm: v.guest, confirm_self: v.confirmSelf || undefined, reason: v.note ?? "" },
      ),
    onSuccess: (_d, v) => {
      toast.info(`${v.guest} was removed`, "That is what the hypervisor reported after the call.");
      void qc.invalidateQueries({ queryKey: ["hv", base, "guests"] });
    },
    onSettled: () => void qc.invalidateQueries({ queryKey: ["infra", "operations"] }),
    onError: (e) => reportError("Couldn't remove the guest", e),
  });
}

export function useCreateGuest(id: string) {
  const qc = useQueryClient();
  return useMutation({
    onMutate: () => showStarted(qc),
    mutationFn: ({ note, ...body }: NewGuest & { note?: string }): Promise<Guest> =>
      callAction(
        "shippin.infrastructure.server.guest.create",
        { serverId: id },
        { ...body, kind: body.kind as "vm" | "ct", reason: note ?? "" },
      ),
    onSuccess: (g) => {
      toast.info(`Guest ${g.id} is ${g.status}`, "The task finished and inventory was read again.");
      void qc.invalidateQueries({ queryKey: ["hv", `/v1/infra/servers/${id}`, "guests"] });
    },
    onSettled: () => void qc.invalidateQueries({ queryKey: ["infra", "operations"] }),
    onError: (e) => reportError("Couldn't create the guest", e),
  });
}

export function useSnapshot(base: string) {
  const qc = useQueryClient();
  return useMutation({
    onMutate: () => showStarted(qc),
    mutationFn: (v: { guest: string; snapname: string; note?: string }) =>
      apiSend<Guest>("POST", `${base}/guests/${v.guest}/snapshots`, { snapname: v.snapname }, { note: v.note }),
    onSuccess: (_g, v) => {
      toast.info("Snapshot saved", v.snapname);
      void qc.invalidateQueries({ queryKey: ["hv", base, v.guest, "snapshots"] });
    },
    onError: (e) => reportError("Couldn't snapshot", e),
  });
}

export function useRollback(base: string) {
  const qc = useQueryClient();
  return useMutation({
    onMutate: () => showStarted(qc),
    mutationFn: (v: { guest: string; name: string; confirmSelf?: boolean; note?: string }) =>
      apiSend<Guest>("POST", `${base}/guests/${v.guest}/snapshots/${v.name}/rollback${v.confirmSelf ? "?confirm=self" : ""}`, undefined, {
        note: v.note,
      }),
    onSuccess: (g) => {
      toast.info(`Rolled back ${g.name || g.id}`, "The guest was read back after the task.");
      void qc.invalidateQueries({ queryKey: ["hv", base, "guests"] });
    },
    onError: (e) => reportError("Couldn't roll back", e),
  });
}
