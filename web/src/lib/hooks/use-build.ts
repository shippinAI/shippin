import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import type { AgentView, PromptCreated, PromptOptions } from "../api/build-types";
import { toast } from "../design/components/toast/toast-queue";

const busy = new Set(["queued", "provisioning", "starting", "running", "waiting_ai"]);

export const usePromptOptions = (tenantId?: string) =>
  useQuery({
    queryKey: ["tenant", tenantId, "prompt-options"],
    queryFn: () => apiGet<PromptOptions>(`/v1/tenants/${tenantId}/prompt/options`),
    enabled: !!tenantId,
    staleTime: 60_000,
  });

export type PromptBody = { prompt: string; provider: string; stack?: string };

/** M4: create a project from a prompt; the build runs as a job with the user's own agent CLI. */
export function useCreateFromPrompt(tenantId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: PromptBody) => apiSend<PromptCreated>("POST", `/v1/tenants/${tenantId}/projects/from-prompt`, body),
    onSuccess: (p) => {
      toast.success(`Building ${p.name}`, "Your devspace is on its way. Follow along in Build.");
      void qc.invalidateQueries({ queryKey: ["tenant", tenantId, "projects"] });
    },
    onError: (e) => toast.error("Couldn't start the build", e.message),
  });
}

/** The project's latest build and its live state; polls while something is happening. */
export const useAgent = (tenantId?: string, projectId?: string) =>
  useQuery({
    queryKey: ["tenant", tenantId, "projects", projectId, "agent"],
    queryFn: () => apiGet<AgentView>(`/v1/tenants/${tenantId}/projects/${projectId}/agent`),
    enabled: !!tenantId && !!projectId,
    refetchInterval: (q) => (q.state.data?.run && busy.has(q.state.data.run.state) ? 4000 : false),
  });

export function useStartAgent(tenantId: string, projectId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (resume: boolean) => apiSend("POST", `/v1/tenants/${tenantId}/projects/${projectId}/agent/start`, { resume }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["tenant", tenantId, "projects", projectId, "agent"] }),
    onError: (e) => toast.error("The build didn't start", e.message),
  });
}
