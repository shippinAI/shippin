import { Plus, Server } from "lucide-react";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { DataTable } from "../../../lib/design/components/data-table/data-table";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import { useServices } from "../../../lib/hooks/use-api";
import { PageHeader } from "../../../template/page-header";
import { instanceColumns, type InstanceRow } from "./instance-columns";

export function InstancesPage() {
  const q = useServices();
  const rows: InstanceRow[] = (q.data?.instances ?? []).map((i) => ({ ...i, id: i.name }));
  const running = rows.filter((r) => r.status === "Running").length;
  return (
    <>
      <PageHeader
        eyebrow="Operator"
        title="Instances"
        description={`Read-only view of the Incus project ${q.data?.project ?? ""} on ${q.data?.host ?? "the host"}.`}
        actions={
          <Button variant="primary" size="sm" isDisabled tooltip="Operator writes go through jobs in v0.2; this view is read-only">
            <Plus className="size-4" /> Launch instance
          </Button>
        }
      />
      <Card elevation={2} className="overflow-hidden">
        <CardHeader
          title={`${rows.length} instances`}
          description={q.data ? `${running} running · refreshes every 15 s` : "Loading…"}
        />
        {q.isPending ? (
          <div className="space-y-3 p-5">
            {Array.from({ length: 4 }, (_, i) => <Skeleton key={i} className="h-9" />)}
          </div>
        ) : (
          <DataTable
            label="Instances"
            columns={instanceColumns}
            rows={rows}
            empty={
              <EmptyState
                illustration={<Server className="size-10 text-fg-subtle" />}
                title={q.data?.instances_error ? "Can't reach Incus" : "No instances"}
                description={q.data?.instances_error ?? q.error?.message ?? "This project has no instances yet."}
              />
            }
          />
        )}
      </Card>
    </>
  );
}
