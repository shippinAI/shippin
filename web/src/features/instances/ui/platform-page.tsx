import { useMe } from "../../../lib/hooks/use-api";
import { PageHeader } from "../../../template/page-header";
import { InfrastructureSummary } from "../../infrastructure/ui/infrastructure-summary";
import { ServicesGrid } from "../../overview/ui/services-grid";
import { TenantsCard } from "./tenants-card";
import { RuntimeDependencies } from "./runtime-dependencies";
import { InternalOrganizationCard } from "../../orgs/ui/internal-organization-card";
import { Button } from "../../../lib/design/components/button/button";
import { useNavigate } from "../../../lib/router/router";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { useStaffMode } from "../../../lib/hooks/use-staff-mode";
import { StaffModeBanner } from "../../admin/ui/staff-mode/staff-mode-banner";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";

export function PlatformPage() {
  const me = useMe().data;
  const nav = useNavigate();
  const text = useText();
  const mode = useStaffMode();
  if (mode.isPending) return <Skeleton className="h-40" />;
  if (mode.data?.required && !mode.data.active) return <StaffModeBanner />;
  return (
    <>
      {mode.data?.active && <StaffModeBanner />}
      <PageHeader eyebrow="Operator" title="Platform health" description="The hosts this panel manages, and the services you have told it to watch." />
      {me?.capabilities["provider.panel"] && <>
        <InternalOrganizationCard />
        <div className="mb-6 flex flex-wrap gap-3">
          <Button tooltip={text("platform.staff")} onPress={() => nav("/admin/team")}>{text("platform.staff")}</Button>
          <Button tooltip={text("platform.organizations")} onPress={() => nav("/admin/tenants")}>{text("platform.organizations")}</Button>
          <Button tooltip={text("platform.catalog")} onPress={() => nav("/admin/services")}>{text("platform.catalog")}</Button>
          <Button tooltip={text("platform.settings")} onPress={() => nav("/admin/platform")}>{text("platform.settings")}</Button>
        </div>
      </>}
      <RuntimeDependencies />
      {me?.capabilities["infra.hosts"] && <InfrastructureSummary />}
      <ServicesGrid />
      {(me?.capabilities["feature.workspaces"] || me?.capabilities["feature.catalog"]) && <TenantsCard />}
    </>
  );
}
