import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, apiGet, apiSend } from "../api/client";
import { toast } from "../design/components/toast/toast-queue";

export type ServerRef = { id: string; slug: string; label: string; site?: string; kind?: string };

export type Host = {
  id: string;
  slug: string;
  label: string;
  kind: string;
  base_url: string;
  node: string;
  token_id: string;
  token_env: string;
};

export type Guest = { id: string; name: string; kind: string; status: string; node: string; tags?: string };

export type Task = { upid: string; status: string; exitstatus: string; type: string; log?: string[]; done: boolean };

export const useHosts = () =>
  useQuery({ queryKey: ["infra", "servers"], queryFn: () => apiGet<{ items: Host[] }>("/v1/infra/servers") });

export const useRegistry = () =>
  useQuery({ queryKey: ["infra", "registry"], queryFn: () => apiGet<{ items: ServerRef[] }>("/v1/infra/servers/registry") });

export function useAttach() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { id: string; base_url: string; node: string; token_id: string; token_env: string; ca_file?: string }) =>
      apiSend<Host>("PUT", `/v1/infra/servers/${body.id}`, {
        kind: "proxmox", base_url: body.base_url, node: body.node, token_id: body.token_id, token_env: body.token_env, ca_file: body.ca_file ?? "",
      }),
    onSuccess: () => {
      toast.info("Hypervisor attached", "The token secret stays in the environment variable you named.");
      void qc.invalidateQueries({ queryKey: ["infra", "servers"] });
      void qc.invalidateQueries({ queryKey: ["infra", "registry"] });
    },
    onError: (e) => toast.error(...hvError("Couldn't attach the hypervisor", e)),
  });
}

export const useGuests = (base: string) =>
  useQuery({
    queryKey: ["hv", base, "guests"],
    queryFn: () => apiGet<{ items: Guest[] }>(`${base}/guests`),
    enabled: !!base,
    refetchInterval: 15_000,
  });

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

function hvError(fallback: string, e: Error) {
  if (e instanceof ApiError && e.code === "waiting_on_board") return ["Approve the card, then try again", e.message] as const;
  return [fallback, e.message] as const;
}

export function useGuestAction(base: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { guest: string; verb: "start" | "stop" | "shutdown" | "reboot" }) =>
      apiSend<Guest>("POST", `${base}/guests/${v.guest}/status/${v.verb}`),
    onSuccess: (g) => {
      toast.info(`${g.name || g.id} is ${g.status}`, "That is the status the hypervisor reported after the call.");
      void qc.invalidateQueries({ queryKey: ["hv", base, "guests"] });
    },
    onError: (e) => toast.error(...hvError("The hypervisor did not change the guest", e)),
  });
}

export function useCreateGuest(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { kind: string; id: string; name: string }) =>
      apiSend<Guest>("POST", `/v1/infra/servers/${id}/guests`, body),
    onSuccess: (g) => {
      toast.info(`Guest ${g.id} is ${g.status}`, "The task finished and inventory was read again.");
      void qc.invalidateQueries({ queryKey: ["hv", `/v1/infra/servers/${id}`, "guests"] });
    },
    onError: (e) => toast.error(...hvError("Couldn't create the guest", e)),
  });
}

export function useSnapshot(base: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { guest: string; snapname: string }) =>
      apiSend<Guest>("POST", `${base}/guests/${v.guest}/snapshots`, { snapname: v.snapname }),
    onSuccess: (_g, v) => {
      toast.info("Snapshot saved", v.snapname);
      void qc.invalidateQueries({ queryKey: ["hv", base, v.guest, "snapshots"] });
    },
    onError: (e) => toast.error(...hvError("Couldn't snapshot", e)),
  });
}

export function useRollback(base: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { guest: string; name: string }) =>
      apiSend<Guest>("POST", `${base}/guests/${v.guest}/snapshots/${v.name}/rollback`),
    onSuccess: (g) => {
      toast.info(`Rolled back ${g.name || g.id}`, "The guest was read back after the task.");
      void qc.invalidateQueries({ queryKey: ["hv", base, "guests"] });
    },
    onError: (e) => toast.error(...hvError("Couldn't roll back", e)),
  });
}
