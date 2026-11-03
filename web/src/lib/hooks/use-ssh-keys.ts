import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import type { SSHKey } from "../api/types";
import { toast } from "../design/components/toast/toast-queue";

export const useSSHKeys = () =>
  useQuery({ queryKey: ["me", "ssh-keys"], queryFn: () => apiGet<{ items: SSHKey[] }>("/v1/me/ssh-keys") });

export function useAddSSHKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { name: string; public_key: string }) =>
      apiSend<SSHKey>("POST", "/v1/me/ssh-keys", { Name: body.name, PublicKey: body.public_key }),
    onSuccess: (k) => {
      toast.success(`Key "${k.name}" added`, "It's being added to your running workspaces.");
      void qc.invalidateQueries({ queryKey: ["me", "ssh-keys"] });
    },
    onError: (e) => toast.error("Key not added", e.message),
  });
}

export function useDeleteSSHKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (k: SSHKey) => apiSend("DELETE", `/v1/me/ssh-keys/${k.id}`),
    onSuccess: (_d, k) => {
      toast.success(`Key "${k.name}" removed`, "It's being removed from your workspaces.");
      void qc.invalidateQueries({ queryKey: ["me", "ssh-keys"] });
    },
    onError: (e) => toast.error("Key not removed", e.message),
  });
}
