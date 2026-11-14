import { Pencil, Plus } from "lucide-react";
import { useState } from "react";
import type { CoverageTier, Plan } from "../../../../lib/api/catalog-types";
import { Badge } from "../../../../lib/design/components/badge/badge";
import { Button } from "../../../../lib/design/components/button/button";
import { IconButton } from "../../../../lib/design/components/button/icon-button";
import { Card, CardHeader } from "../../../../lib/design/components/card/card";
import { useCoverageTiers, usePlans } from "../../../../lib/hooks/use-catalog";
import { CoverageSummary } from "../../../coverage/ui/coverage-summary";
import { signInLabel } from "../../../coverage/wording";
import { CoverageForm } from "./coverage-form";
import { CatalogPerm, useCan } from "../../../../lib/hooks/use-can";
import { PageHeader } from "../../../../template/page-header";
import { QueryView } from "../query-view";
import { GuidedEmpty } from "./guided-empty";
import { PlanForm } from "./plan-form";
import { ServiceMatrix } from "./service-matrix";
import { TermTip } from "./term-tip";

/** "$1500.00/mo minimum or 10% of plan charges": whichever is greater applies. */
const coveragePrice = (cents: number, pct: number) =>
  [cents > 0 && `$${(cents / 100).toFixed(2)}/mo minimum`, pct > 0 && `${pct}% of plan charges`].filter(Boolean).join(" or ");
const price = (c: number) => (c === 0 ? "Free / custom" : `$${(c / 100).toFixed(2)}/mo`);

export function PlansPage() {
  const q = usePlans();
  const [edit, setEdit] = useState<Partial<Plan> | null>(null);
  const [editTier, setEditTier] = useState<Partial<CoverageTier> | null>(null);
  const tiers = useCoverageTiers().data?.items ?? [];
  const tierLabel = (id: string) => tiers.find((t) => t.id === id)?.label ?? id;
  const plans = q.data?.items ?? [];
  const can = useCan();
  const write = can(CatalogPerm.plansWrite);
  return (
    <>
      <PageHeader eyebrow="Provider" title="Plans & tiers" description="Plans sell capacity and services. Coverage tiers sell support and operation beside them: each plan includes one, and a customer can be moved up. All of it is yours to rename, reprice or change."
        actions={write && <Button variant="primary" size="sm" tooltip="Add a tier" onPress={() => setEdit({})}><Plus className="size-4" /> New plan</Button>} />
      <QueryView isPending={q.isPending} error={q.error} onRetry={() => void q.refetch()}>
        {() => plans.length === 0 ? (
          <Card><GuidedEmpty title="No plans yet" action={write && <Button tooltip="Create a plan" variant="primary" size="sm" onPress={() => setEdit({})}>Create a plan</Button>}
            steps={["Create a tier such as free or pro.", "Tick services in the matrix and set limits.", "Assign the plan to customers."]} /></Card>
        ) : (
          <>
            <Card className="mb-6">
              <CardHeader title="Coverage tiers" description="Support hours, response targets, who operates the platform, and professional services: consulting, custom deployments, remote operations and training."
                actions={write && <Button size="sm" tooltip="Add a coverage tier" onPress={() => setEditTier({})}><Plus className="size-4" /> New coverage tier</Button>} />
              <div className="grid gap-4 p-5 lg:grid-cols-3">
                {tiers.map((t) => (
                  <div key={t.id} className="rounded-lg border border-line p-4">
                    <div className="flex items-start justify-between gap-2">
                      <div><div className="font-semibold text-fg">{t.label}</div><div className="font-mono text-xs text-fg-subtle">{t.id}</div></div>
                      {write && <IconButton icon={Pencil} label={`Edit ${t.label}`} onPress={() => setEditTier(t)} />}
                    </div>
                    <p className="mt-2 text-sm text-fg-muted">{t.description}</p>
                    <div className="mt-3 flex flex-wrap gap-1.5">
                      <Badge tone={t.sign_in === "password" ? "neutral" : "info"}>Sign-in: {signInLabel[t.sign_in]}</Badge>
                      {t.price_cents != null && (t.price_cents > 0 || (t.spend_percent ?? 0) > 0) && <Badge tone="primary">{coveragePrice(t.price_cents, t.spend_percent ?? 0)}</Badge>}
                      {!t.public && <Badge>sales only</Badge>}
                      {!t.active && <Badge tone="warning">inactive</Badge>}
                    </div>
                    <div className="mt-3"><CoverageSummary tier={t} compact /></div>
                  </div>
                ))}
              </div>
            </Card>
            <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
              {plans.map((p) => (
                <Card key={p.id} className="p-5">
                  <div className="flex items-start justify-between gap-2">
                    <div><div className="font-semibold text-fg">{p.label}</div><div className="font-mono text-xs text-fg-subtle">{p.id}</div></div>
                    {write && <IconButton icon={Pencil} label={`Edit ${p.label}`} onPress={() => setEdit(p)} />}
                  </div>
                  <p className="mt-2 text-sm text-fg-muted">{p.description}</p>
                  <div className="mt-3 flex flex-wrap gap-1.5">
                    {p.price_cents !== undefined && <Badge tone="primary">{price(p.price_cents)}</Badge>}
                    {p.coverage && <Badge tone="info">Includes {tierLabel(p.coverage)}</Badge>}
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
      {editTier && <CoverageForm initial={editTier.id ? (editTier as CoverageTier) : undefined} onClose={() => setEditTier(null)} />}
      {edit && <PlanForm initial={edit.id ? (edit as Plan) : undefined} onClose={() => setEdit(null)} />}
    </>
  );
}
