import { Boxes, CircleCheck, CircleStop, Network } from "lucide-react";
import { useState } from "react";
import { Card } from "../../../lib/design/components/card/card";
import { ConfirmDialog } from "../../../lib/design/components/dialog/confirm-dialog";
import { DataTable } from "../../../lib/design/components/data-table/data-table";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import type { AdminWorkspace } from "../../../lib/api/admin-types";
import { useAdminWorkspaces } from "../../../lib/hooks/use-admin";
import { useAdminWorkspaceAction } from "../../../lib/hooks/use-admin-actions";
import { PageHeader } from "../../../template/page-header";
import { QueryView } from "./query-view";
import { StatCard } from "./stat-card";
import { TableToolbar, matches } from "./table-toolbar";
import { workspaceColumns } from "./workspace-columns";
import { AccessDialogs } from "./access/access-launcher";

export function AdminWorkspacesPage() {
  const q = useAdminWorkspaces();
  const act = useAdminWorkspaceAction();
  const [search, setSearch] = useState("");
  const [doomed, setDoomed] = useState<AdminWorkspace>();
  const [ask, setAsk] = useState<{ w: AdminWorkspace; mode: "request" | "breakglass" }>();
  const all = q.isError ? [] : q.data?.items ?? [];
  const rows = all.filter((w) => matches(search, w.name, w.tenant_name, w.created_by, w.ipv4, w.host, w.ssh_port, w.state));
  const count = (s: string) => (q.data && !q.isError ? all.filter((w) => w.state === s).length : undefined);
  const cols = workspaceColumns((w, a) => {
    if (a === "delete") setDoomed(w);
    else if (a === "request" || a === "breakglass") setAsk({ w, mode: a });
    else act.mutate({ id: w.id, name: w.name, action: a });
  }, act.isPending);
  return (
    <>
      <PageHeader eyebrow="Provider" title="Workspaces" description="All workspaces across tenants with live Incus state. Opening or acting on one needs the customer's approval." />
      {all.some((w) => w.observation && w.observation !== "observed") && <p role="status">Some workspace states could not be verified. Running and stopped counts include verified observations only.</p>}
      <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatCard label="Workspaces" value={q.data && !q.isError ? all.length : undefined} icon={Boxes} hue="var(--color-hue-workspaces)" hint="Not deleted" />
        <StatCard label="Running" value={count("running")} icon={CircleCheck} hue="var(--color-success)" hint="Live in Incus" />
        <StatCard label="Stopped" value={count("stopped")} icon={CircleStop} hue="var(--color-fg-muted)" hint="Start from the row" />
        <StatCard label="Tenants with workspaces" value={q.data && !q.isError ? new Set(all.map((w) => w.tenant_id)).size : undefined} icon={Network} hue="var(--color-primary)" />
      </div>
      <Card className="overflow-hidden">
        <TableToolbar label="workspaces" value={search} onChange={setSearch} count={rows.length} />
        <QueryView isPending={q.isPending} error={q.error} onRetry={() => void q.refetch()}>
          {() => (
            <DataTable
              label="Workspaces"
              dense
              columns={cols}
              rows={rows}
              empty={<EmptyState title={search ? "No matching workspaces" : "No workspaces yet"} description="Workspaces appear here once a tenant creates one." />}
            />
          )}
        </QueryView>
      </Card>
      {ask && <AccessDialogs tenantId={ask.w.tenant_id} tenantName={ask.w.tenant_name} workspaceId={ask.w.id} mode={ask.mode} onClose={() => setAsk(undefined)} />}
      <ConfirmDialog
        isOpen={!!doomed}
        onOpenChange={(o) => !o && setDoomed(undefined)}
        title={`Delete ${doomed?.name ?? "workspace"}?`}
        body={`The workspace in ${doomed?.tenant_name ?? "its tenant"}, its home volume, HTTPS and preview routes and SSH port are removed. This can't be undone.`}
        confirmLabel="Delete workspace"
        typeToConfirm={doomed?.name}
        onConfirm={() => doomed && act.mutate({ id: doomed.id, name: doomed.name, action: "delete", confirm: doomed.name })}
      />
    </>
  );
}
