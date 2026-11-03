import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { Button } from "../../../lib/design/components/button/button";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import { useDecideAccess, useTenantAccess, useTenantAudit } from "../../../lib/hooks/use-access";
import { AuditTable } from "./audit-table";
import { GrantStatusBadge, grantScope, humanDuration, untilLabel } from "./grant-meta";

/** Tenant settings: platform access grants (revoke any time) and the tenant's audit log. */
export function AccessGrantsCard({ tenantId }: { tenantId: string }) {
  const q = useTenantAccess(tenantId);
  const audit = useTenantAudit(tenantId);
  const decide = useDecideAccess(tenantId);
  const items = q.data?.items ?? [];
  const active = items.filter((g) => g.status === "approved");
  const history = items.filter((g) => g.status !== "approved" && g.status !== "pending").slice(0, 8);
  return (
    <>
      <Card>
        <CardHeader title="Platform access" description="Shippin admins can open your workspaces only while you've approved a time-boxed grant. Break-glass (emergencies) is listed here too." />
        {q.isPending ? (
          <Skeleton className="m-5 h-16" />
        ) : active.length + history.length === 0 ? (
          <EmptyState title="No platform access" description="When an admin asks for access you'll see a banner with Approve and Deny." />
        ) : (
          <ul className="divide-y divide-line">
            {[...active, ...history].map((g) => (
              <li key={g.id} className="flex flex-wrap items-center gap-x-4 gap-y-1 px-5 py-3.5 text-[13px]">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2 font-medium text-fg">{g.requester_name} <GrantStatusBadge grant={g} /></div>
                  <div className="mt-0.5 truncate text-fg-muted" title={g.reason}>
                    {grantScope(g)} · {humanDuration(g.duration_s)} · {g.status === "approved" ? untilLabel(g.expires_at) : new Date(g.created_at).toLocaleDateString()} · “{g.reason}”
                  </div>
                </div>
                {g.status === "approved" && g.kind === "request" && (
                  <Button size="sm" variant="danger" isDisabled={decide.isPending} tooltip="End this admin's access now" onPress={() => decide.mutate({ grant: g, decision: "revoke" })}>
                    Revoke
                  </Button>
                )}
                {g.status === "approved" && g.kind === "breakglass" && (
                  <span className="text-xs text-fg-subtle">Ends automatically</span>
                )}
              </li>
            ))}
          </ul>
        )}
      </Card>
      <Card className="overflow-hidden">
        <CardHeader title="Audit log" description="Every request, decision, expiry and admin access to your project space. Append-only: entries can't be changed or deleted." />
        {audit.isPending ? <Skeleton className="m-5 h-24" /> : <AuditTable items={audit.data?.items ?? []} />}
      </Card>
    </>
  );
}
