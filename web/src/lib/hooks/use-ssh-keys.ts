import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { callAction } from "../api/actions";
import type { SSHKey } from "../api/types";
import { toast } from "../design/components/toast/toast-queue";

export const useSSHKeys = () =>
  useQuery({ queryKey: ["me", "ssh-keys"], queryFn: () => callAction<{ items: SSHKey[] }>("shippin.identity.ssh_key.list") });

export function useAddSSHKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { name: string; public_key: string }) => callAction<SSHKey>("shippin.identity.ssh_key.create", {}, body),
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
    mutationFn: (k: SSHKey) => callAction("shippin.identity.ssh_key.delete", { sshKeyId: k.id }),
    onSuccess: (_d, k) => {
      toast.success(`Key "${k.name}" removed`, "It's being removed from your workspaces.");
      void qc.invalidateQueries({ queryKey: ["me", "ssh-keys"] });
    },
    onError: (e) => toast.error("Key not removed", e.message),
  });
}
