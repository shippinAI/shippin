import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../../lib/api/client";

export type AISession = { id: string; label: string; running: boolean; created?: number; pane_pid?: number };

export const useAISessions = () =>
  useQuery({ queryKey: ["ai", "sessions"], queryFn: () => apiGet<{ sessions: AISession[] }>("/v1/ai/sessions"), refetchInterval: 10_000 });

export function useLaunch() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiSend<AISession>("POST", `/v1/ai/sessions/${encodeURIComponent(id)}?rows=40&cols=140`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["ai", "sessions"] }),
  });
}

export function useStop() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiSend("DELETE", `/v1/ai/sessions/${encodeURIComponent(id)}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["ai", "sessions"] }),
  });
}

export function attachURL(loc: Location, id: string, rows: number, cols: number) {
  const proto = loc.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${loc.host}/v1/ai/sessions/${encodeURIComponent(id)}/attach?rows=${rows}&cols=${cols}`;
}
