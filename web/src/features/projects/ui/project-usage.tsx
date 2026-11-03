import { Gauge } from "lucide-react";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import type { Project } from "../../../lib/api/project-types";
import { bytes, useUsage } from "../../../lib/hooks/use-usage";

const usd = (n: number) => `$${n.toFixed(2)}`;

/** Resources and cost per project, from the tenant's Incus metrics. "Shared" is the boundary VM. */
export function ProjectUsage({ tenantId, projects }: { tenantId: string; projects: Project[] }) {
  const q = useUsage(tenantId);
  const name = (id: string) => (id ? (projects.find((p) => p.id === id)?.name ?? id.slice(0, 8)) : "Shared (devspace VM)");
  const rows = q.data?.projects ?? [];
  const total = rows.reduce((s, p) => s + p.monthly_usd, 0);
  return (
    <Card>
      <CardHeader title="Usage & cost" description={rows.length ? `About ${usd(total)}/month at current use.` : "Live CPU, memory, disk and traffic per project."} />
      {q.isError && <p className="px-5 pb-4 text-[13px] text-fg-muted">Usage is not available right now.</p>}
      <ul className="divide-y divide-line">
        {rows.map((p) => (
          <li key={p.project_id || "shared"} className="px-5 py-3 text-[13px]">
            <div className="flex items-center gap-2 font-medium text-fg">
              <Gauge className="size-4 text-fg-subtle" /> {name(p.project_id)}
              <span className="flex-1" />
              <span title="Memory and disk held for a month">{usd(p.monthly_usd)}/mo</span>
            </div>
            <div className="mt-1 flex flex-wrap gap-x-4 text-xs text-fg-subtle">
              <span>CPU {(p.cpu_seconds / 3600).toFixed(2)} h</span>
              <span>RAM {bytes(p.memory_bytes)}</span>
              <span>Disk {bytes(p.disk_bytes)}</span>
              <span>Out {bytes(p.net_tx_bytes)}</span>
              <span title="CPU used and traffic since start">used {usd(p.accrued_usd)}</span>
              <span>{p.instances.length} instance{p.instances.length === 1 ? "" : "s"}</span>
            </div>
          </li>
        ))}
      </ul>
    </Card>
  );
}
