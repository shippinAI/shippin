import { Pencil, Plus } from "lucide-react";
import { useState } from "react";
import type { Plan } from "../../../../lib/api/catalog-types";
import { Badge } from "../../../../lib/design/components/badge/badge";
import { Button } from "../../../../lib/design/components/button/button";
import { IconButton } from "../../../../lib/design/components/button/icon-button";
import { Card, CardHeader } from "../../../../lib/design/components/card/card";
import { usePlans } from "../../../../lib/hooks/use-catalog";
import { CatalogPerm, useCan } from "../../../../lib/hooks/use-can";
import { PageHeader } from "../../../../template/page-header";
import { QueryView } from "../query-view";
import { GuidedEmpty } from "./guided-empty";
import { PlanForm } from "./plan-form";
import { ServiceMatrix } from "./service-matrix";
import { TermTip } from "./term-tip";

const price = (c: number) => (c === 0 ? "Free / custom" : `$${(c / 100).toFixed(2)}/mo`);

export function PlansPage() {
  const q = usePlans();
  const [edit, setEdit] = useState<Partial<Plan> | null>(null);
  const plans = q.data?.items ?? [];
  const can = useCan();
  const write = can(CatalogPerm.plansWrite);
  return (
    <>
      <PageHeader eyebrow="Provider" title="Plans & tiers" description="Tiers are data: rename, reprice or add one. The matrix sets which services each tier includes and their limits."
        actions={write && <Button variant="primary" size="sm" tooltip="Add a tier" onPress={() => setEdit({})}><Plus className="size-4" /> New plan</Button>} />
      <QueryView isPending={q.isPending} error={q.error} onRetry={() => void q.refetch()}>
        {() => plans.length === 0 ? (
          <Card><GuidedEmpty title="No plans yet" action={write && <Button tooltip="Create a plan" variant="primary" size="sm" onPress={() => setEdit({})}>Create a plan</Button>}
            steps={["Create a tier such as free or pro.", "Tick services in the matrix and set limits.", "Assign the plan to customers."]} /></Card>
        ) : (
          <>
            <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
              {plans.map((p) => (
                <Card key={p.id} className="p-5">
                  <div className="flex items-start justify-between gap-2">
                    <div><div className="font-semibold text-fg">{p.label}</div><div className="font-mono text-[11.5px] text-fg-subtle">{p.id}</div></div>
                    {write && <IconButton icon={Pencil} label={`Edit ${p.label}`} onPress={() => setEdit(p)} />}
                  </div>
                  <p className="mt-2 text-[13px] text-fg-muted">{p.description}</p>
                  <div className="mt-3 flex flex-wrap gap-1.5">
                    {p.price_cents !== undefined && <Badge tone="primary">{price(p.price_cents)}</Badge>}
                    {!p.public && <Badge>sales only</Badge>}
                    {!p.active && <Badge tone="warning">inactive</Badge>}
                  </div>
                </Card>
              ))}
            </div>
            <Card className="mt-6 overflow-hidden">
              <CardHeader title={<span className="inline-flex items-center gap-1.5">Service matrix <TermTip term="matrix" /></span>}
                description={write ? "Click a cell to include a service, set its lock mode and limits." : "Read only: changing the matrix needs catalog.plans.write."} />
              <ServiceMatrix plans={plans} canEdit={write} />
            </Card>
          </>
        )}
      </QueryView>
      {edit && <PlanForm initial={edit.id ? (edit as Plan) : undefined} onClose={() => setEdit(null)} />}
    </>
  );
}
