import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import type { Agent, AgentList } from "../api/agent-types";
import { toast } from "../design/components/toast/toast-queue";

export const useAgents = (tenantId?: string) =>
  useQuery({
    queryKey: ["agents", tenantId],
    queryFn: () => apiGet<AgentList>(`/v1/tenants/${tenantId}/identity/agents`),
    enabled: !!tenantId,
    retry: false,
  });

/** Create an agent identity (an IdP service user delegated by you). No credential is returned. */
export function useCreateAgent(tenantId?: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { name: string; workspace_id?: string }) =>
      apiSend<Agent>("POST", `/v1/tenants/${tenantId}/identity/agents`, body),
    onSuccess: (a) => {
      toast.success(`Agent "${a.name}" created`, "It has its own identity; it never uses your sign-in.");
      void qc.invalidateQueries({ queryKey: ["agents", tenantId] });
    },
    onError: (e) => toast.error("Agent not created", e.message),
  });
}
