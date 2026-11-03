import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import type { PermGrants } from "../api/team-types";
import { toast } from "../design/components/toast/toast-queue";

const path = "/v1/operator/team/permission-grants";

export const usePermGrants = () =>
  useQuery({ queryKey: ["admin", "perm-grants"], queryFn: () => apiGet<PermGrants>(path), refetchInterval: 30_000, retry: false });

function useGrantSend<V>(send: (v: V) => Promise<unknown>, done: string, failed: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: send,
    onSuccess: () => toast.success(done),
    onError: (e) => toast.error(failed, e.message),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ["admin"] });
      void qc.invalidateQueries({ queryKey: ["me"] });
    },
  });
}

export type GrantBody = { subject: string; permission: string; reason: string };
export const useGrantPerm = () => useGrantSend((v: GrantBody) => apiSend("POST", path, v), "Permission granted (audited)", "Not granted");

export const useRevokePerm = () =>
  useGrantSend((v: { subject: string; permission: string; reason: string }) =>
    apiSend("DELETE", `${path}/${encodeURIComponent(v.subject)}/${encodeURIComponent(v.permission)}?reason=${encodeURIComponent(v.reason)}`),
  "Permission revoked (audited)", "Not revoked");
