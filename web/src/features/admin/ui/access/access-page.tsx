import { ShieldCheck, ShieldQuestion, Siren, TimerOff } from "lucide-react";
import { Card, CardHeader } from "../../../../lib/design/components/card/card";
import { DataTable } from "../../../../lib/design/components/data-table/data-table";
import { EmptyState } from "../../../../lib/design/components/empty-state/empty-state";
import { useAdminAccess } from "../../../../lib/hooks/use-admin";
import { useAdminAudit, useAdminGrants, useEndGrant } from "../../../../lib/hooks/use-admin-access";
import { PageHeader } from "../../../../template/page-header";
import { AuditTable } from "../../../access/ui/audit-table";
import { QueryView } from "../query-view";
import { StatCard } from "../stat-card";
import { AuditChainBadge } from "./audit-chain-badge";
import { BreakGlassApprovals } from "./breakglass-approvals";
import { grantColumns } from "./grant-columns";

/** Provider: every access request / grant with its status, and the full audit log. */
export function AccessPage() {
  const acc = useAdminAccess().data;
  const me = acc?.user_id ?? "";
  const perms = acc?.staff?.permissions ?? [];
  const q = useAdminGrants();
  const audit = useAdminAudit();
  const end = useEndGrant();
  const all = q.data?.items ?? [];
  const count = (f: (s: string, k: string) => boolean) => (q.data ? all.filter((g) => f(g.status, g.kind)).length : undefined);
  return (
    <>
      <PageHeader eyebrow="Provider" title="Access & audit" description="Customer data is off-limits unless the customer approves a time-boxed grant. Break-glass is for emergencies and is always audited." />
      <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatCard label="Pending" value={count((s) => s === "pending")} icon={ShieldQuestion} hue="var(--color-warning)" hint="Waiting for the customer" />
        <StatCard label="Active" value={count((s, k) => s === "approved" && k === "request")} icon={ShieldCheck} hue="var(--color-success)" hint="Customer-approved" />
        <StatCard label="Break-glass active" value={count((s, k) => s === "approved" && k === "breakglass")} icon={Siren} hue="var(--color-danger)" hint="Emergency access" />
        <StatCard label="Closed" value={count((s) => s !== "approved" && s !== "pending")} icon={TimerOff} hue="var(--color-fg-muted)" hint="Denied, revoked, expired, ended" />
      </div>
      <BreakGlassApprovals enabled={perms.includes("access.approve_breakglass")} />
      <Card className="mb-6 overflow-hidden">
        <CardHeader title="Requests and grants" description="Request access from a tenant or workspace in Tenants / Workspaces." />
        <QueryView isPending={q.isPending} error={q.error} onRetry={() => void q.refetch()}>
          {() => (
            <DataTable label="Access grants" dense columns={grantColumns(me, (g) => end.mutate(g), end.isPending)} rows={all}
              empty={<EmptyState title="No access requests yet" description="Open a tenant and choose Request access." />} />
          )}
        </QueryView>
      </Card>
      <Card className="overflow-hidden">
        <CardHeader title="Audit log" description="All tenants, newest first. Append-only and hash-chained; break-glass entries are red."
          actions={<AuditChainBadge chain={audit.data?.chain} />} />
        <QueryView isPending={audit.isPending} error={audit.error} onRetry={() => void audit.refetch()}>
          {() => <AuditTable items={audit.data?.items ?? []} showTenant />}
        </QueryView>
      </Card>
    </>
  );
}
