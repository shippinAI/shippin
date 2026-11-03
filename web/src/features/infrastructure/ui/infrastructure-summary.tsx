import { useInfraOverview } from "../../../lib/hooks/use-infra";
import { ProviderCards } from "./provider-cards";

/** The provider cards on the pages an owner lands on. It adds nothing until there is data to show. */
export function InfrastructureSummary() {
  const q = useInfraOverview();
  if (!q.data) return null;
  return <div className="mb-8"><ProviderCards providers={q.data.providers} /></div>;
}
