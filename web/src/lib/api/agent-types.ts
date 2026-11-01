// Agent identities (/v1/tenants/{id}/identity/agents; spec schemas/nomen.yaml#/Agent).
export type Agent = {
  id: string; name: string; workspace_id?: string; delegating_user_id: string; idp_user_id: string;
  tool_grants: string[]; created_at: string;
};

export type AgentList = { items: Agent[]; page: { next_page_token: string | null; total_estimate: number | null }; enabled: boolean };
