import { useQuery } from "@tanstack/react-query";
import { callAction } from "../../lib/api/actions";
import type { CoverageTier } from "../../lib/api/catalog-types";

/** What a member's own organization is covered for (no prices). */
export type OwnCoverage = Omit<CoverageTier, "rank" | "sign_in" | "price_cents" | "spend_percent" | "public" | "active">;

export const useOwnCoverage = (tenantId?: string) =>
  useQuery({
    queryKey: ["coverage", tenantId],
    queryFn: () => callAction<{ coverage: OwnCoverage | null }>("shippin.tenancy.coverage.get", { tenantId: tenantId! }),
    enabled: !!tenantId,
  });
