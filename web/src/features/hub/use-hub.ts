import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../../lib/api/client";
import type { HubEvent, HubMe, HubTask } from "./hub-types";

export const useHubMe = () => useQuery({ queryKey: ["hub", "me"], queryFn: () => apiGet<HubMe>("/v1/hub/me"), retry: false });

/** Pushed by useHubLive; the slow poll only covers a dropped stream. */
export const useHubTasks = (live: boolean) =>
  useQuery({ queryKey: ["hub", "tasks"], queryFn: () => apiGet<{ items: HubTask[] }>("/v1/hub/tasks"),
    refetchInterval: live ? 60_000 : 5_000, retry: false });

export const useHubEvents = (id: string, enabled: boolean) =>
  useQuery({ queryKey: ["hub", "events", id], enabled, retry: false,
    queryFn: () => apiGet<{ items: HubEvent[] }>(`/v1/hub/tasks/${encodeURIComponent(id)}/events`) });

export function useCreateTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (b: { title: string; body: string }) => apiSend<HubTask>("POST", "/v1/hub/tasks", b),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["hub", "tasks"] }),
  });
}

export function useDecide() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (d: { id: string; approve: boolean; note?: string }) =>
      apiSend<HubTask>("POST", `/v1/hub/tasks/${encodeURIComponent(d.id)}/decision`, { approve: d.approve, note: d.note ?? "" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["hub", "tasks"] }),
  });
}
