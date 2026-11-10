import { CheckCircle2, Circle, CircleSlash, XCircle } from "lucide-react";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import type { SetupState } from "../../../lib/api/setup-types";
import { useProvisionRRFlow, useSetup } from "../../../lib/hooks/use-setup";

const icon = { ok: CheckCircle2, todo: Circle, off: CircleSlash, error: XCircle };
const color = { ok: "text-success", todo: "text-fg-subtle", off: "text-fg-subtle", error: "text-danger" };
const label: Record<SetupState, string> = { ok: "Done", todo: "To do", off: "Not enabled", error: "Failed" };

/** Setup checklist: org, tenant VM, rrflow enabled, and rrflow state per project, each from the
 *  server's real state. Tenants can provision missing rrflow; the provider panel reads only. */
export function SetupChecklist({ tenantId, scope }: { tenantId: string; scope: "tenant" | "operator" }) {
  const q = useSetup(tenantId, scope);
  const fix = useProvisionRRFlow(tenantId);
  const checks = q.data?.checks ?? [];
  const canFix = scope === "tenant" && checks.some((c) => c.fix === "rrflow_attach");
  return (
    <Card>
      <CardHeader
        title="Setup checklist"
        description={q.data ? `${q.data.done} of ${q.data.total} done` : q.isError ? "Setup state is unavailable." : "Checking…"}
        actions={canFix && (
          <Button size="sm" variant="secondary" tooltip="Provision rrflow state for every project missing it" isDisabled={fix.isPending} onPress={() => fix.mutate()}>
            Provision rrflow
          </Button>
        )}
      />
      <ul className="divide-y divide-line" aria-label="Setup checks">
        {checks.map((c) => {
          const I = icon[c.state];
          return (
            <li key={c.id} className="flex items-start gap-3 px-4 py-2.5">
              <I className={`mt-0.5 size-4 shrink-0 ${color[c.state]}`} aria-hidden />
              <div className="min-w-0 flex-1">
                <p className="font-medium text-fg">{c.label}</p>
                {c.detail && <p className="text-base text-fg-muted">{c.detail}</p>}
              </div>
              <Badge tone={c.state === "ok" ? "success" : c.state === "error" ? "danger" : c.state === "todo" ? "warning" : "neutral"}>{label[c.state]}</Badge>
            </li>
          );
        })}
      </ul>
      {fix.isError && <p role="alert" className="px-4 pb-3 text-base text-danger">{fix.error.message}</p>}
    </Card>
  );
}
