import { Plus } from "lucide-react";
import { useNavigate } from "../../../lib/router/router";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { DataTable } from "../../../lib/design/components/data-table/data-table";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import { useStateToasts } from "../../../lib/hooks/use-state-toasts";
import { useTenant } from "../../../lib/hooks/use-tenant";
import { useWorkspaces } from "../../../lib/hooks/use-workspaces";
import { PageHeader } from "../../../template/page-header";
import { workspaceColumns } from "./workspace-columns";

export function DevspacesPage() {
  const { tenant } = useTenant();
  const q = useWorkspaces(tenant?.tenant_id);
  const nav = useNavigate();
  useStateToasts(q.isError ? undefined : q.data?.items);
  const items = q.isError ? [] : q.data?.items ?? [];
  const can = !q.isError && !!q.data?.can_manage && q.data.tenant_status === "active";
  const why = !q.data?.can_manage ? "Needs org_owner or org_admin" : "Tenant isn't provisioned yet";
  const createBtn = (
    <Button variant="primary" size="sm" isDisabled={!can} tooltip={can ? "Create a new dev workspace" : why} onPress={() => nav(`/t/${tenant?.tenant_id}/workspaces/new`)}>
      <Plus className="size-4" /> New devspace
    </Button>
  );
  const running = items.filter((w) => w.state === "running").length;
  return (
    <>
      <PageHeader eyebrow={tenant?.name} title="Devspaces" description="Isolated dev servers with a browser IDE, their own encrypted storage and a public HTTPS address." actions={createBtn} />
      {q.isError && <p role="alert">Devspace inventory could not be loaded. <Button tooltip="Reload devspace inventory" onPress={() => void q.refetch()}>Retry</Button></p>}
      {items.some((w) => w.observation && w.observation !== "observed") && <p role="status">Some devspace states could not be verified. Recorded state is not live health.</p>}
      {!q.isError && <Card elevation={2} className="overflow-hidden">
        <CardHeader title={`${items.length} devspace${items.length === 1 ? "" : "s"}`} description={q.data ? `${running} running` : "LoadingÃ¢â‚¬Â¦"} />
        {q.isPending ? (
          <div className="space-y-3 p-5">{Array.from({ length: 3 }, (_, i) => <Skeleton key={i} className="h-11" />)}</div>
        ) : (
          <DataTable
            label="Devspaces"
            columns={workspaceColumns(!!q.data?.can_manage)}
            rows={items}
            empty={<EmptyState title="No devspaces yet" description="Create a development instance on configured infrastructure." action={createBtn} />}
          />
        )}
      </Card>}
    </>
  );
}
