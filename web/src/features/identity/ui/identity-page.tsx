import { Bot } from "lucide-react";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { DataTable, type ColumnDef } from "../../../lib/design/components/data-table/data-table";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import type { Agent } from "../../../lib/api/agent-types";
import { timeAgo } from "../../../lib/format";
import { useAgents } from "../../../lib/hooks/use-agents";
import { useTenant } from "../../../lib/hooks/use-tenant";
import { useWorkspaces } from "../../../lib/hooks/use-workspaces";
import { PageHeader } from "../../../template/page-header";
import { QueryView } from "../../admin/ui/query-view";
import { CreateAgentForm } from "./create-agent-form";

/** Identity (Nomen): agent identities of this tenant (customer apps follow in the same page). */
export function IdentityPage() {
  const { tenant } = useTenant();
  const q = useAgents(tenant?.tenant_id);
  const ws = useWorkspaces(tenant?.tenant_id).data?.items ?? [];
  const wsName = (id?: string) => (id ? (ws.find((w) => w.id === id)?.name ?? id.slice(0, 8)) : "Any");
  const cols: ColumnDef<Agent>[] = [
    { id: "name", label: "Agent", isRowHeader: true, render: (a) => <span className="flex items-center gap-2 font-medium text-fg"><Bot className="size-4 text-hue-identity" />{a.name}</span> },
    { id: "ws", label: "Workspace", render: (a) => wsName(a.workspace_id) },
    { id: "idp", label: "Service user", render: (a) => <span className="font-mono text-sm">{a.idp_user_id}</span> },
    { id: "tools", label: "Tool grants", render: (a) => (a.tool_grants.length ? a.tool_grants.join(", ") : "None yet") },
    { id: "created", label: "Created", render: (a) => timeAgo(a.created_at) },
  ];
  const enabled = q.data?.enabled ?? false;
  const canCreate = enabled && !!tenant?.can_manage;
  const reason = !enabled ? "Agent identities aren't enabled on this platform yet" : "Needs the org owner or admin role";
  return (
    <>
      <PageHeader eyebrow="Identity" title="Agents" description="Each agent is its own service user in your organization, delegated by a person. Agents never use your sign-in or your tokens." />
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[1.6fr_1fr]">
        <Card elevation={2} className="overflow-hidden">
          <CardHeader title="Agent identities" />
          <QueryView isPending={q.isPending} error={q.error} onRetry={() => void q.refetch()}>
            {() => (
              <DataTable label="Agent identities" columns={cols} rows={q.data?.items ?? []}
                empty={<EmptyState title="No agents yet" description="Create one for a build bot or a coding agent in a workspace." />} />
            )}
          </QueryView>
        </Card>
        {tenant && <CreateAgentForm tenantId={tenant.tenant_id} canCreate={canCreate} reason={reason} />}
      </div>
    </>
  );
}
