import { ShieldAlert, ShieldQuestion, Siren } from "lucide-react";
import { useEffect, useRef } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { toast } from "../../../lib/design/components/toast/toast-queue";
import { useDecideAccess, useTenantAccess } from "../../../lib/hooks/use-access";
import { useTenant } from "../../../lib/hooks/use-tenant";
import { grantScope, humanDuration, untilLabel } from "./grant-meta";

/** Customer console banner: pending admin access requests (approve/deny) and break-glass notices.
 *  Admins viewing a tenant through a grant see a reminder instead. */
export function AccessBanner() {
  const { tenant } = useTenant();
  const isDecider = !!tenant?.can_manage && !tenant.access;
  const q = useTenantAccess(tenant?.tenant_id, isDecider);
  const decide = useDecideAccess(tenant?.tenant_id);
  const pending = (q.data?.items ?? []).filter((g) => g.status === "pending");
  const seen = useRef(new Set<string>());
  useEffect(() => {
    for (const g of pending) {
      if (!seen.current.has(g.id)) {
        toast.info("Access request", `${g.requester_name} is requesting access to your project space.`);
        seen.current.add(g.id);
      }
    }
  }, [pending]);
  if (tenant?.access)
    return (
      <div role="status" className="mb-6 flex items-center gap-3 rounded-xl border border-warning/30 bg-warning/10 px-4 py-3 text-sm">
        {tenant.access.kind === "breakglass" ? <Siren className="size-4 text-danger" /> : <ShieldAlert className="size-4 text-warning" />}
        <span className="text-fg">
          You're viewing {tenant.name} through {tenant.access.kind === "breakglass" ? "break-glass" : "customer-approved"} access ({tenant.access.scope}), {untilLabel(tenant.access.expires_at)}. Every open and action is audited.
        </span>
      </div>
    );
  if (!isDecider) return null;
  return (
    <div className="mb-6 space-y-3">
      {q.data?.notices.map((g) => (
        <div key={g.id} role="alert" className="flex gap-3 rounded-xl border border-danger/35 bg-danger/10 px-4 py-3.5 text-sm">
          <Siren className="mt-0.5 size-4 shrink-0 text-danger" />
          <div className="min-w-0">
            <div className="font-semibold text-fg">Emergency access by Shippin admin {g.requester_name}</div>
            <div className="mt-0.5 text-fg-muted">
              {new Date(g.created_at).toLocaleString()} · {grantScope(g)} · {g.status === "approved" ? untilLabel(g.expires_at) : g.status}. Reason: {g.reason}
            </div>
            <div className="mt-1 text-xs text-fg-subtle">Break-glass is reserved for security incidents, legal or abuse reports and outages. Every access is in your audit log (Settings).</div>
          </div>
        </div>
      ))}
      {pending.map((g) => (
        <div key={g.id} role="alert" className="flex flex-wrap items-center gap-4 rounded-xl border border-primary/35 bg-primary/10 px-4 py-3.5 shadow-e2">
          <ShieldQuestion className="size-5 shrink-0 text-primary" />
          <div className="min-w-0 flex-1 text-sm">
            <div className="text-base font-semibold text-fg">Admin {g.requester_name} is requesting access to your project space</div>
            <div className="mt-0.5 text-fg-muted">
              <span className="text-fg">{grantScope(g)}</span> · for {humanDuration(g.duration_s)} · “{g.reason}”
            </div>
          </div>
          <div className="flex gap-2">
            <Button size="sm" variant="ghost" isDisabled={decide.isPending} tooltip="Refuse; the admin keeps no access" onPress={() => decide.mutate({ grant: g, decision: "deny" })}>
              Deny
            </Button>
            <Button size="sm" variant="primary" isDisabled={decide.isPending} tooltip={`Grant access for ${humanDuration(g.duration_s)}; you can revoke it any time in Settings`} onPress={() => decide.mutate({ grant: g, decision: "approve" })}>
              Approve
            </Button>
          </div>
        </div>
      ))}
    </div>
  );
}
