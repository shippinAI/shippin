import { Pencil, Plus } from "lucide-react";
import { useState } from "react";
import type { RunnerSize } from "../../../../lib/api/actions.gen";
import { Badge } from "../../../../lib/design/components/badge/badge";
import { Button } from "../../../../lib/design/components/button/button";
import { IconButton } from "../../../../lib/design/components/button/icon-button";
import { Card } from "../../../../lib/design/components/card/card";
import { CatalogPerm, useCan } from "../../../../lib/hooks/use-can";
import { useRunnerSizes } from "../../../../lib/hooks/use-catalog";
import { PageHeader } from "../../../../template/page-header";
import { QueryView } from "../query-view";
import { GuidedEmpty } from "./guided-empty";
import { RunnerSizeForm } from "./runner-size-form";
import { sizeSummary } from "./runner-sizes";

/** The runner sizes customers' GitHub Actions jobs can ask for (RUN-01). A plan includes some of them
 * through the runners service in the plan matrix. */
export function RunnerSizesPage() {
  const write = useCan()(CatalogPerm.plansWrite);
  const q = useRunnerSizes();
  const [edit, setEdit] = useState<{ size?: RunnerSize } | null>(null);
  const items = q.data?.items ?? [];
  return (
    <>
      <PageHeader eyebrow="Provider" title="Runner sizes"
        description="The machines customers' build jobs run on. A job asks for one with runs-on: shippin-<id>; each plan's Build runners cell in Plans & tiers says which sizes it includes, how many jobs run at once and the minutes a month."
        actions={write && <Button variant="primary" size="sm" tooltip="Offer a new runner size" onPress={() => setEdit({})}><Plus className="size-4" /> New size</Button>} />
      <QueryView isPending={q.isPending} error={q.error} onRetry={() => void q.refetch()}>
        {() => items.length === 0 ? (
          <Card><GuidedEmpty title="No runner sizes yet" action={write && <Button tooltip="Offer a runner size" variant="primary" size="sm" onPress={() => setEdit({})}>Add a size</Button>}
            steps={["Add a size with its cores, memory, disk and runner image.", "In Plans & tiers, include Build runners on a plan and list the size in its sizes.", "Customers' jobs ask for it with runs-on: shippin-<id>."]} /></Card>
        ) : (
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            {items.map((s) => (
              <Card key={s.id} className="flex flex-col p-5">
                <div className="flex items-start justify-between gap-2">
                  <div><div className="font-semibold text-fg">{s.label}</div><div className="font-mono text-xs text-fg-subtle">runs-on: shippin-{s.id}</div></div>
                  {write && <IconButton icon={Pencil} label={`Edit ${s.label}`} onPress={() => setEdit({ size: s })} />}
                </div>
                <p className="mt-2 text-sm text-fg-muted">{sizeSummary(s)}</p>
                <div className="mt-3 flex flex-wrap gap-1.5">
                  {s.active ? <Badge tone="success">offered</Badge> : <Badge tone="warning">off</Badge>}
                  {s.image ? <Badge tone="info">image set</Badge> : <Badge tone="danger">no image: jobs refused as not ready</Badge>}
                </div>
                {s.image && <div className="mt-auto truncate pt-3 font-mono text-2xs text-fg-subtle" title={s.image}>{s.image}</div>}
              </Card>
            ))}
          </div>
        )}
      </QueryView>
      {!write && q.isSuccess && <p className="mt-4 text-sm text-fg-muted">Read only: changing runner sizes needs catalog.plans.write.</p>}
      {edit && <RunnerSizeForm initial={edit.size} onClose={() => setEdit(null)} />}
    </>
  );
}
