import { PageHeader } from "../../../template/page-header";
import { ServicesGrid } from "../../overview/ui/services-grid";
import { TenantsCard } from "./tenants-card";

export function PlatformPage() {
  return (
    <>
      <PageHeader eyebrow="Operator" title="Platform health" description="Live probes of every platform service, joined with Incus state." />
      <ServicesGrid />
      <TenantsCard />
    </>
  );
}
