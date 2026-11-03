import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../../../lib/api/client";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";

type Item = { id: string; name: string; role: string; state: "ready" | "missing" | "not_available"; detail?: string };
type Bundle = { release: string; items: Item[] };

const tone = { ready: "success", missing: "warning", not_available: "neutral" } as const;
const label = { ready: "Ready", missing: "Missing", not_available: "Not available yet" };

/** The Bundled Stack every tenant ships with, with each component's real state. */
export function BundleCard({ tenantId }: { tenantId: string }) {
  const qc = useQueryClient();
  const key = ["bundle", tenantId];
  const q = useQuery({ queryKey: key, queryFn: () => apiGet<Bundle>(`/v1/tenants/${tenantId}/bundle`) });
  const fix = useMutation({
    mutationFn: () => apiSend<Bundle>("POST", `/v1/tenants/${tenantId}/bundle/reconcile`),
    onSuccess: (b) => qc.setQueryData(key, b),
  });
  const items = q.data?.items ?? [];
  return (
    <Card>
      <CardHeader
        title="Bundled stack"
        description={q.data ? `Every tenant ships with the same stack, pinned to ${q.data.release}.` : "Every tenant ships with the same stack."}
        actions={<Button size="sm" variant="secondary" tooltip="Provision anything missing" isDisabled={fix.isPending} onPress={() => fix.mutate()}>Reconcile</Button>}
      />
      <ul className="divide-y divide-border">
        {items.map((i) => (
          <li key={i.id} className="flex items-start justify-between gap-3 px-4 py-2.5">
            <div className="min-w-0">
              <p className="font-medium text-fg">{i.name}</p>
              <p className="text-sm text-fg-muted">{i.role}{i.detail ? ` · ${i.detail}` : ""}</p>
            </div>
            <Badge tone={tone[i.state]}>{label[i.state]}</Badge>
          </li>
        ))}
      </ul>
    </Card>
  );
}
