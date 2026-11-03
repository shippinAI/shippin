import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import type { Preview, PreviewURL } from "../api/types";
import { toast } from "../design/components/toast/toast-queue";

export type ListenPort = { port: number; local_only: boolean };

/** Ports the workspace currently listens on (polled while the tab is open). */
export const useListening = (tenantId: string, id: string, enabled: boolean) =>
  useQuery({
    queryKey: ["tenant", tenantId, "ws", id, "listening"],
    queryFn: () => apiGet<{ items: ListenPort[]; note?: string }>(`/v1/tenants/${tenantId}/workspaces/${id}/listening`),
    enabled,
    refetchInterval: 5000,
  });

/** Replace the workspace's preview ports; the edge routes are synced server-side. */
export function useSetPreviews(tenantId: string, id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ previews }: { previews: Preview[]; message: string }) =>
      apiSend<PreviewURL[]>("PUT", `/v1/tenants/${tenantId}/workspaces/${id}/previews`, { previews }),
    onSuccess: (_d, { message }) => toast.success(message),
    onError: (e) => toast.error("Preview not updated", e.message),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ["tenant", tenantId, "workspaces"] });
    },
  });
}
