import { Check, Minus } from "lucide-react";
import { useState } from "react";
import { Button } from "react-aria-components";
import type { Plan, PlanService } from "../../../../lib/api/catalog-types";
import { usePlanServices, useServices } from "../../../../lib/hooks/use-catalog";
import { QueryView } from "../query-view";
import { MatrixCellDialog } from "./matrix-cell-dialog";

const brief = (l: PlanService["limits"]) => Object.entries(l).map(([k, v]) => `${k} ${v}`).join(" · ");

/** Services (rows) x plans (columns); each cell shows inclusion, lock and limits. */
export function ServiceMatrix({ plans, canEdit }: { plans: Plan[]; canEdit: boolean }) {
  const svcs = useServices();
  const cells = usePlanServices();
  const [open, setOpen] = useState<{ plan: Plan; service: string; cell?: PlanService } | null>(null);
  const find = (p: string, s: string) => cells.data?.items.find((c) => c.plan_id === p && c.service_id === s);
  return (
    <>
    <QueryView isPending={svcs.isPending || cells.isPending} error={svcs.error ?? cells.error} onRetry={() => { void svcs.refetch(); void cells.refetch(); }}>
      {() => (
        <div className="overflow-x-auto">
          <table className="w-full min-w-180 text-base">
            <thead><tr className="text-left text-xs tracking-wide text-fg-subtle uppercase">
              <th className="px-5 py-2.5 font-medium">Service</th>
              {plans.map((p) => <th key={p.id} className="px-3 py-2.5 font-medium">{p.label}</th>)}
            </tr></thead>
            <tbody>
              {(svcs.data?.items ?? []).map((s) => (
                <tr key={s.id} className="border-t border-line align-top">
                  <th scope="row" className="px-5 py-3 text-left font-medium text-fg">{s.label}
                    {s.required && <div className="text-2xs font-normal text-fg-subtle">required</div>}</th>
                  {plans.map((p) => {
                    const c = find(p.id, s.id);
                    return (
                      <td key={p.id} className="px-2 py-2">
                        <Button aria-label={`${s.label} on ${p.label}`} isDisabled={!canEdit} onPress={() => setOpen({ plan: p, service: s.id, cell: c })}
                          className="focus-ring w-full rounded-lg border border-line px-2.5 py-2 text-left data-hovered:border-line-strong">
                          <div className="flex items-center gap-1.5">
                            {c?.enabled ? <Check className="size-4 text-success" /> : <Minus className="size-4 text-fg-subtle" />}
                            <span className="text-xs text-fg-muted">{c ? c.lock : "not included"}</span>
                          </div>
                          {c && <div className="mt-1 text-2xs text-fg-subtle">{brief(c.limits) || "no limits"}</div>}
                        </Button>
                      </td>
                    );
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </QueryView>
    {open && <MatrixCellDialog plan={open.plan} service={open.service} cell={open.cell} onClose={() => setOpen(null)} />}
    </>
  );
}
