import { AlertTriangle, Boxes, Building2, CircleCheck, PauseCircle } from "lucide-react";
import { useState } from "react";
import { Card } from "../../../lib/design/components/card/card";
import { DataTable } from "../../../lib/design/components/data-table/data-table";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { useAdminTenants } from "../../../lib/hooks/use-admin";
import { PageHeader } from "../../../template/page-header";
import { QueryView } from "./query-view";
import { StatCard } from "./stat-card";
import { TableToolbar, matches } from "./table-toolbar";
import { tenantColumns } from "./tenant-columns";
import { TenantDrawer } from "./tenant-drawer";

export function TenantsPage() {
  const q = useAdminTenants();
  const [search, setSearch] = useState("");
  const [open, setOpen] = useState<string>();
  const all = q.data?.items ?? [];
  const rows = all.filter((t) => matches(search, t.name, t.id, t.owner, t.owner_email, t.subnet, t.status));
  const n = (s: string) => (q.data ? all.filter((t) => t.status === s).length : undefined);
  return (
    <>
      <PageHeader eyebrow="Provider" title="Tenants" description="Every organization on the platform, its network silo and lifecycle." />
      <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-5">
        <StatCard label="Tenants" value={q.data ? all.length : undefined} icon={Building2} hue="var(--color-primary)" hint="All statuses" />
        <StatCard label="Active" value={n("active")} icon={CircleCheck} hue="var(--color-success)" hint="Serving workspaces" />
        <StatCard label="Suspended" value={n("suspended")} icon={PauseCircle} hue="var(--color-warning)" hint="Workspaces stopped" />
        <StatCard label="Failed" value={n("failed")} icon={AlertTriangle} hue="var(--color-danger)" hint="Need a retry" />
        <StatCard label="Workspaces" value={q.data ? all.reduce((s, t) => s + t.workspace_count, 0) : undefined} icon={Boxes} hue="var(--color-hue-workspaces)" hint="Across tenants" />
      </div>
      {q.data?.warning && <div className="mb-3 rounded-lg border border-warning/30 bg-warning/10 px-4 py-2 text-[13px] text-warning">{q.data.warning}</div>}
      <Card className="overflow-hidden">
        <TableToolbar label="tenants" value={search} onChange={setSearch} count={rows.length} />
        <QueryView isPending={q.isPending} error={q.error} onRetry={() => void q.refetch()}>
          {() => (
            <DataTable
              label="Tenants"
              dense
              columns={tenantColumns}
              rows={rows}
              onRowAction={setOpen}
              empty={<EmptyState title={search ? "No matching tenants" : "No tenants yet"} description={search ? "Try another name, id or owner." : "Tenants appear when a customer signs up or you create one."} />}
            />
          )}
        </QueryView>
      </Card>
      <TenantDrawer tenant={all.find((t) => t.id === open)} onClose={() => setOpen(undefined)} />
    </>
  );
}
