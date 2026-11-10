import { Loader2, RotateCw } from "lucide-react";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { DataTable, type ColumnDef } from "../../../lib/design/components/data-table/data-table";
import { timeAgo } from "../../../lib/format";
import { useAllTenants, useRetryProvision, type OperatorTenant } from "../../../lib/hooks/use-provision";

const tone = (s: string): "success" | "danger" | "info" => (s === "active" ? "success" : s === "failed" ? "danger" : "info");

/** Operator: every tenant with status, the raw provisioning error and a Retry. */
export function TenantsCard() {
  const q = useAllTenants(true);
  const retry = useRetryProvision();
  const cols: ColumnDef<OperatorTenant>[] = [
    { id: "name", label: "Tenant", isRowHeader: true, render: (t) => <span><span className="block font-medium text-fg">{t.name}</span><span className="font-mono text-xs text-fg-subtle">{t.id}</span></span> },
    { id: "status", label: "Status", render: (t) => <Badge tone={tone(t.status)}>{t.status}</Badge> },
    { id: "err", label: "Last error", render: (t) => <span className="line-clamp-2 font-mono text-xs text-danger" title={t.error}>{t.error || "—"}</span> },
    { id: "created", label: "Created", render: (t) => timeAgo(t.created_at) },
    {
      id: "x", label: "", width: "120px",
      render: (t) => (
        <Button size="sm" variant={t.status === "failed" ? "primary" : "ghost"} isDisabled={retry.isPending} tooltip="Re-run idempotent provisioning" onPress={() => retry.mutate({ id: t.id, name: t.name })}>
          {retry.isPending && retry.variables?.id === t.id ? <Loader2 className="size-4 animate-spin" /> : <RotateCw className="size-4" />} Retry
        </Button>
      ),
    },
  ];
  return (
    <Card elevation={2} className="mt-8 overflow-hidden">
      <CardHeader title="Tenants" description="Provisioning status across the platform. Errors are shown to operators only." />
      <DataTable label="Tenants" columns={cols} rows={q.data?.items ?? []} empty={<p className="p-5 text-base text-fg-muted">No tenants yet.</p>} />
    </Card>
  );
}
