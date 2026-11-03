import { AlertTriangle } from "lucide-react";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import { timeAgo } from "../../../lib/format";
import { useServices } from "../../../lib/hooks/use-api";
import { ServiceCard } from "./service-card";

/** Live platform service cards; polls every 15 s. */
export function ServicesGrid() {
  const q = useServices();
  return (
    <section>
      <div className="mb-3.5 flex items-baseline justify-between">
        <h2 className="text-[15px] font-semibold">Platform services</h2>
        <span className="text-xs text-fg-subtle">
          {q.data ? `Live · ${q.data.host} / ${q.data.project} · checked ${timeAgo(q.data.checked_at)}` : "Checking…"}
        </span>
      </div>
      {q.isError && (
        <div className="flex items-center gap-2 rounded-lg border border-danger/30 bg-danger/10 px-4 py-3 text-sm text-danger">
          <AlertTriangle className="size-4" /> {q.error.message}
        </div>
      )}
      {q.data?.probes.some((p) => p.configured === false) && (
        <p className="mb-3.5 text-[13px] text-fg-muted" data-testid="probes-not-set-up">
          Services marked Not set up have no address to check yet. Add one under Provider panel, Platform settings.
        </p>
      )}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {q.isPending &&
          Array.from({ length: 4 }, (_, i) => <Skeleton key={i} className="h-[196px] rounded-[var(--radius-card)]" />)}
        {q.data?.probes.map((p) => (
          <ServiceCard key={p.name} probe={p} instance={q.data.instances.find((i) => i.name === p.instance)} />
        ))}
      </div>
    </section>
  );
}
