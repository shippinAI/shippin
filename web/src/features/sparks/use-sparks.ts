import { useQuery } from "@tanstack/react-query";
import { apiGet } from "../../lib/api/client";

export type SparkEndpoint = { name: string; state: "blocked" | "published"; url?: string; reason?: string };
type SparkNode = { id: string; name: string; role: string; management: SparkEndpoint[] };
type Sparks = { nodes: SparkNode[]; model_api?: SparkEndpoint };

export function useSparks(tenantId: string) {
  return useQuery({
    queryKey: ["tenant", tenantId, "sparks"],
    queryFn: () => apiGet<Sparks>(`/v1/tenants/${tenantId}/sparks`),
    staleTime: 30_000,
  });
}
