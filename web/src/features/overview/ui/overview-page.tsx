import { Activity, Boxes, Server, Users } from "lucide-react";
import { LinkButton } from "../../../lib/design/components/button/link-button";
import { useServices } from "../../../lib/hooks/use-api";
import { useWorkspaces } from "../../../lib/hooks/use-workspaces";
import { useTenant } from "../../../lib/hooks/use-tenant";
import { PageHeader } from "../../../template/page-header";
import { TenantOnboardingCard } from "../../go/ui/tenant-onboarding-card";
import { SetupChecklist } from "../../setup/ui/setup-checklist";
import { GettingStarted } from "./getting-started";
import { SetupBanner } from "./setup-banner";
import { ServicesGrid } from "./services-grid";
import { StatTile } from "./stat-tile";
import { WorkspacesEmpty } from "./workspaces-empty";
import { SparkFleet } from "../../sparks/spark-fleet";
import { InfrastructureSummary } from "../../infrastructure/ui/infrastructure-summary";
import { useText } from "../../../lib/ui-config/use-ui-config";

export function OverviewPage() {
  const { tenant, tenants, me, isOperator } = useTenant();
  const svc = useServices(isOperator).data;
  const healthy = svc?.probes.filter((p) => p.healthy).length;
  const running = svc?.instances.filter((i) => i.status === "Running").length;
  const wss = useWorkspaces(tenant?.tenant_id).data?.items;
  const first = me?.display_name.split(" ")[0] ?? "";
  const t = useText();
  return (
    <>
      <PageHeader
        eyebrow={tenant?.name}
        title={`Good to see you${first ? `, ${first}` : ""}`}
        description="Here's what's happening across your tenant and the platform it runs on."
        actions={
          tenant && (
            <LinkButton href={`/t/${tenant.tenant_id}/settings`} tooltip="Open tenant settings" size="sm">
              Tenant settings
            </LinkButton>
          )
        }
      />
      {tenant && <SetupBanner tenant={tenant} />}
      {tenant && wss?.length === 0 && <div className="mb-8"><TenantOnboardingCard tenantId={tenant.tenant_id} /></div>}
      {tenant && !tenant.access && <div className="mb-8"><SetupChecklist tenantId={tenant.tenant_id} scope="tenant" /></div>}
      <div className="mb-8 grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatTile label="Services healthy" value={isOperator ? (svc ? `${healthy}/${svc.probes.length}` : undefined) : "—"} hint="Live probes, 15 s" icon={Activity} hue="var(--color-success)" />
        <StatTile label="Instances running" value={isOperator ? running : "—"} hint={svc ? `Project ${svc.project}` : t("home.instances.hint")} icon={Server} hue="var(--color-hue-edge)" />
        <StatTile label="Workspaces" value={wss?.length} hint={wss ? `${wss.filter((w) => w.state === "running").length} running` : "Loading"} icon={Boxes} hue="var(--color-hue-workspaces)" />
        <StatTile label="Tenants" value={tenants.length} hint="You're a member of" icon={Users} hue="var(--color-hue-identity)" />
      </div>
      {me?.capabilities["infra.hosts"] && <InfrastructureSummary />}
      {isOperator && <ServicesGrid />}
      {tenant && !tenant.access && me?.capabilities["host.admin_tools"] && <SparkFleet tenantId={tenant.tenant_id} />}
      <div className="mt-8 grid grid-cols-1 gap-6 lg:grid-cols-[1.4fr_1fr]">
        <WorkspacesEmpty tenantId={tenant?.tenant_id} />
        <GettingStarted
          steps={[
            { title: "Create your tenant", body: `${tenant?.name ?? "Your org"} is live in the identity provider.`, done: !!tenant },
            { title: "Sign in with OIDC", body: "PKCE login through your org's identity.", done: !!me },
            { title: "Invite your team", body: "Members and roles arrive with the Team module.", done: false },
            { title: "Launch a workspace", body: "A dev box with git, Go, Node and SSH.", done: !!wss?.length },
          ]}
        />
      </div>
    </>
  );
}
