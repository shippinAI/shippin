import { useQuery } from "@tanstack/react-query";
import { callAction } from "../../lib/api/actions";
import type { CoverageTierCustomerView } from "../../lib/api/actions.gen";

/** What a member's own organization is covered for (no prices). */
export type OwnCoverage = CoverageTierCustomerView;

export const useOwnCoverage = (tenantId?: string) =>
  useQuery({
    queryKey: ["coverage", tenantId],
    queryFn: () => callAction("shippin.tenancy.coverage.get", { tenantId: tenantId! }),
    enabled: !!tenantId,
  });
