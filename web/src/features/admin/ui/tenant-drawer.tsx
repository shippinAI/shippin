import { CopyField } from "../../../lib/design/components/field/copy-field";
import { Drawer } from "../../../lib/design/components/drawer/drawer";
import type { AdminTenant } from "../../../lib/api/admin-types";
import { StatusBadge } from "./status-badge";
import { TenantActions } from "./tenant-actions";
import { AccessLauncher } from "./access/access-launcher";
import { TenantVMCard } from "./tenant-vm/tenant-vm-card";
import { TenantPeople } from "./tenant-people";
import { SetupChecklist } from "../../setup/ui/setup-checklist";
import { useIdpName } from "../../../lib/hooks/use-idp-name";

/** Tenant detail sheet: identifiers, network, owner, raw error, lifecycle actions. */
export function TenantDrawer({ tenant: t, onClose }: { tenant?: AdminTenant; onClose: () => void }) {
  const idp = useIdpName();
  return (
    <Drawer
      isOpen={!!t}
      onOpenChange={(o) => !o && onClose()}
      title={t?.name ?? ""}
      subtitle={t && <span className="inline-flex items-center gap-2"><StatusBadge status={t.status} /> {t.plan_id} plan</span>}
      footer={t && <TenantActions tenant={t} onDeleted={onClose} />}
    >
      {t && (
        <div>
          {t.error && (
            <div className="m-5 mb-0 rounded-lg border border-danger/30 bg-danger/10 px-4 py-3 text-[13px]">
              <div className="font-medium text-danger">Last error (operator view)</div>
              <pre className="mt-1 font-mono text-[12px] whitespace-pre-wrap text-fg-muted">{t.error}</pre>
            </div>
          )}
          <div className="mt-3 [&>div]:grid-cols-[140px_1fr]">
            <CopyField label="Tenant UUID" value={t.id} mono />
            <CopyField label={`${idp} org`} value={t.idp_org_id} mono />
            <CopyField label="Owner" value={t.owner ? `${t.owner} <${t.owner_email}>` : ""} />
            <CopyField label="Subnet" value={t.subnet} mono hint={t.router ? `router ${t.router}` : undefined} />
            <CopyField label="SSH endpoint" value={t.ssh_ip ?? ""} mono />
            <CopyField label="Incus project" value={`t-${t.id}`} mono />
            <CopyField label="Created" value={new Date(t.created_at).toLocaleString()} />
          </div>
          <TenantPeople tenantId={t.id} />
          <div className="mx-5 mt-2"><SetupChecklist tenantId={t.id} scope="operator" /></div>
          {t.status !== "deleted" && <TenantVMCard tenant={t} />}
          <div className="mx-5 mt-2 rounded-lg border border-line bg-raised/50 px-4 py-3">
            <div className="text-[13px] font-medium text-fg">Workspace access</div>
            <p className="mt-0.5 text-xs text-fg-muted">Opening this tenant's IDEs or previews, or acting on its workspaces, needs the customer's approval (or audited break-glass).</p>
            <div className="mt-3 flex gap-2"><AccessLauncher tenantId={t.id} tenantName={t.name} disabled={t.status === "deleted"} /></div>
          </div>
          <p className="px-6 py-4 text-xs text-fg-subtle">{t.workspace_count} workspace{t.workspace_count === 1 ? "" : "s"}. Suspending stops them and blocks starts until resumed.</p>
        </div>
      )}
    </Drawer>
  );
}
