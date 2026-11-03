import { useMe } from "../../../lib/hooks/use-api";
import { PageHeader } from "../../../template/page-header";
import { InfrastructureSummary } from "../../infrastructure/ui/infrastructure-summary";
import { ServicesGrid } from "../../overview/ui/services-grid";
import { TenantsCard } from "./tenants-card";

export function PlatformPage() {
  const me = useMe().data;
  return (
    <>
      <PageHeader eyebrow="Operator" title="Platform health" description="Live probes of every platform service, joined with Incus state." />
      {me?.capabilities["infra.hosts"] && <InfrastructureSummary />}
      <ServicesGrid />
      <TenantsCard />
    </>
  );
}
