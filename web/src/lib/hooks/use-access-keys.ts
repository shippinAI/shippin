import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { callAction } from "../api/actions";
import { toast } from "../design/components/toast/toast-queue";

/** An access key as its owner sees it. The key itself is only in the create answer, once. */
export type AccessKey = { id: string; name: string; prefix: string; created_at: string; expires_at: string; last_used_at: string | null };
export type CreatedAccessKey = AccessKey & { token: string };

const key = ["me", "access-keys"];

export const useAccessKeys = () =>
  useQuery({ queryKey: key, queryFn: () => callAction<{ items: AccessKey[] }>("shippin.identity.access_key.list") });

export function useCreateAccessKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { name: string; days: number }) => callAction<CreatedAccessKey>("shippin.identity.access_key.create", {}, body),
    onSuccess: () => void qc.invalidateQueries({ queryKey: key }),
    onError: (e) => toast.error("Key not made", e.message),
  });
}

export function useRevokeAccessKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (k: AccessKey) => callAction("shippin.identity.access_key.revoke", { accessKeyId: k.id }),
    onSuccess: (_d, k) => {
      toast.success(`Key "${k.name}" revoked`, "Anything using it is signed out now.");
      void qc.invalidateQueries({ queryKey: key });
    },
    onError: (e) => toast.error("Key not revoked", e.message),
  });
}
