import { Globe, ShieldCheck } from "lucide-react";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { CopyField } from "../../../lib/design/components/field/copy-field";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import { useTenantSettings } from "../../../lib/hooks/use-api";
import { useTenant } from "../../../lib/hooks/use-tenant";
import { PageHeader } from "../../../template/page-header";
import { AccessGrantsCard } from "../../access/ui/access-grants-card";
import { AppCards } from "../../apps/ui/app-cards";
import { BundleCard } from "./bundle-card";
import { CoverageCard } from "../../coverage/ui/coverage-card";
import { useIdpName } from "../../../lib/hooks/use-idp-name";

export function SettingsPage() {
  const idp = useIdpName();
  const { tenant } = useTenant();
  const q = useTenantSettings(tenant?.tenant_id);
  const s = q.data;
  if (!s || !tenant) return <Skeleton className="h-96" />;
  return (
    <>
      <PageHeader eyebrow={s.display_name} title="Tenant settings" description="Your tenant's identity, plan and domains." />
      <div className="space-y-6">
        <Card>
          <CardHeader title="General" description="Read-only for now; editing arrives with putTenantSettings." />
          <CopyField label="Display name" value={s.display_name} />
          <CopyField label="Tenant ID" value={s.tenant_id} mono hint="Use with the API and CLI" />
          <CopyField label="Plan" value={s.plan_name} />
          <CopyField label="Region" value={s.region} hint="Host running your tenant" />
          <CopyField label="Status" value={s.status} />
        </Card>
        <CoverageCard tenantId={tenant.tenant_id} />
        <Card>
          <CardHeader title="Identity & infrastructure" description="Your tenant is an org in the identity provider with its own isolated project." />
          <CopyField label="OIDC issuer" value={s.idp_issuer} mono />
          <CopyField label="Organization ID" value={s.idp_org_id} mono hint={`The organization in ${idp} mapped to this tenant`} />
          <CopyField label="Incus project" value={s.incus_project} mono />
          <CopyField label="SSH address" value={s.ssh_ip} mono hint="Shared by your workspaces" />
        </Card>
        <Card>
          <CardHeader title="AI apps" description="Copy a command into your own terminal. It installs the app and starts the vendor sign-in." />
          <div className="px-5 py-4"><AppCards tenantId={tenant.tenant_id} /></div>
        </Card>
        <BundleCard tenantId={tenant.tenant_id} />
        <Card>
          <CardHeader title="Domains & governance" description="Only domains configured by the platform appear here." />
          <ul className="divide-y divide-line">
            {s.governance_domains.map((d) => (
              <li key={d} className="flex items-center gap-3 px-5 py-3.5">
                <Globe className="size-4 text-hue-network" />
                <span className="flex-1 font-mono text-sm">{d}</span>
                {d === s.default_domain && <Badge tone="primary">Default</Badge>}
                <Badge tone="success"><ShieldCheck className="size-3" /> Governed</Badge>
              </li>
            ))}
          </ul>
        </Card>
        {tenant.can_manage && !tenant.access && <AccessGrantsCard tenantId={tenant.tenant_id} />}
        <Card className="border-danger/30">
          <CardHeader
            title="Danger zone"
            description="Deleting a tenant removes its workspaces, secrets and identity org."
            actions={
              <Button variant="danger" size="sm" isDisabled tooltip="Done by a platform administrator with a typed-name confirmation">
                Delete tenant
              </Button>
            }
          />
        </Card>
      </div>
    </>
  );
}
