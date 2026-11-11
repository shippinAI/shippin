import { LifeBuoy } from "lucide-react";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { useOwnCoverage } from "../use-coverage";
import { CoverageSummary } from "./coverage-summary";

/** The organization's coverage tier: how to get help and what is done for it. */
export function CoverageCard({ tenantId }: { tenantId: string }) {
  const text = useText();
  const q = useOwnCoverage(tenantId);
  const c = q.data?.coverage;
  return (
    <Card>
      <CardHeader title={<span className="inline-flex items-center gap-2"><LifeBuoy className="size-4 text-primary" />{c ? text("coverage.title", { name: c.label }) : text("coverage.none")}</span>}
        description={c?.description ?? text("coverage.noneDescription")} />
      {q.isPending && <div className="px-5 pb-5"><Skeleton className="h-40" /></div>}
      {q.isError && <p role="alert" className="px-5 pb-5 text-danger">{q.error.message}</p>}
      {c && <div className="px-5 pb-4"><CoverageSummary tier={c} /></div>}
    </Card>
  );
}
