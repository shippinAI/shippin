import { useMe } from "../../../lib/hooks/use-api";
import { PageHeader } from "../../../template/page-header";
import { InfrastructureSummary } from "../../infrastructure/ui/infrastructure-summary";
import { ServicesGrid } from "../../overview/ui/services-grid";
import { TenantsCard } from "./tenants-card";

export function PlatformPage() {
  const me = useMe().data;
  return (
    <>
      <PageHeader eyebrow="Operator" title="Platform health" description="The hosts this panel manages, and the services you have told it to watch." />
      {me?.capabilities["infra.hosts"] && <InfrastructureSummary />}
      <ServicesGrid />
      {(me?.capabilities["feature.workspaces"] || me?.capabilities["feature.catalog"]) && <TenantsCard />}
    </>
  );
}
