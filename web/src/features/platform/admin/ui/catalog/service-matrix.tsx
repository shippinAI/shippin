import { Check, Minus } from "lucide-react";
import { useState } from "react";
import { Button } from "react-aria-components";
import type { Plan, PlanService } from "../../../../../lib/api/catalog-types";
import { useServices } from "../../../../../lib/hooks/use-catalog";
import { QueryView } from "../query-view";
import { MatrixCellDialog } from "./matrix-cell-dialog";
import { runnerLimitsBrief, runnersService } from "./runner-sizes";

const brief = (service: string, l: PlanService["limits"]) =>
  service === runnersService
    ? runnerLimitsBrief(l)
    : Object.entries(l)
        .map(([k, v]) => `${k} ${v}`)
        .join(" · ");

/** Services (rows) x plans (columns); each cell shows inclusion, lock and limits. Each plan carries its own cells. */
export function ServiceMatrix({ plans, canEdit }: { plans: Plan[]; canEdit: boolean }) {
  const svcs = useServices();
  const [open, setOpen] = useState<{ plan: Plan; service: string; cell?: PlanService } | null>(null);
  const find = (p: Plan, s: string) => p.services.find((c) => c.service_id === s);
  return (
    <>
      <QueryView isPending={svcs.isPending} error={svcs.error} onRetry={() => void svcs.refetch()}>
        {() => (
          <div className="overflow-x-auto">
            <table className="w-full min-w-180 text-base">
              <thead>
                <tr className="text-left text-xs tracking-wide text-fg-subtle uppercase">
                  <th className="px-5 py-2.5 font-medium">Service</th>
                  {plans.map((p) => (
                    <th key={p.id} className="px-3 py-2.5 font-medium">
                      {p.label}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {(svcs.data?.items ?? []).map((s) => (
                  <tr key={s.id} className="border-t border-line align-top">
                    <th scope="row" className="px-5 py-3 text-left font-medium text-fg">
                      {s.label}
                      {s.required && <div className="text-2xs font-normal text-fg-subtle">required</div>}
                    </th>
                    {plans.map((p) => {
                      const c = find(p, s.id);
                      return (
                        <td key={p.id} className="px-2 py-2">
                          <Button
                            aria-label={`${s.label} on ${p.label}`}
                            isDisabled={!canEdit}
                            onPress={() => setOpen({ plan: p, service: s.id, cell: c })}
                            className="focus-ring w-full rounded-lg border border-line px-2.5 py-2 text-left data-hovered:border-line-strong"
                          >
                            <div className="flex items-center gap-1.5">
                              {c?.enabled ? <Check className="size-4 text-success" /> : <Minus className="size-4 text-fg-subtle" />}
                              <span className="text-xs text-fg-muted">{c ? c.lock : "not included"}</span>
                            </div>
                            {c && <div className="mt-1 text-2xs text-fg-subtle">{brief(s.id, c.limits) || "no limits"}</div>}
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
