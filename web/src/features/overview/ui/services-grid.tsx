import { Activity, AlertTriangle } from "lucide-react";
import { LinkButton } from "../../../lib/design/components/button/link-button";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import { timeAgo } from "../../../lib/format";
import { useMe, useServices } from "../../../lib/hooks/use-api";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { ServiceCard } from "./service-card";

/** Live platform service cards; polls every 15 s. */
export function ServicesGrid() {
  const q = useServices();
  const t = useText();
  const canConfigure = useMe().data?.capabilities["provider.panel"] === true;
  const joined = (q.data?.instances.length ?? 0) > 0;
  return (
    <section>
      <div className="mb-3.5 flex items-baseline justify-between">
        <h2 className="text-lg font-semibold">{t("services.title")}</h2>
        <span className="text-xs text-fg-subtle">
          {q.data ? [t("services.live"), joined ? `${q.data.host} / ${q.data.project}` : "", t("services.checked", { when: timeAgo(q.data.checked_at) })].filter(Boolean).join(" · ") : t("services.checking")}
        </span>
      </div>
      {q.isError && (
        <div className="flex items-center gap-2 rounded-lg border border-danger/30 bg-danger/10 px-4 py-3 text-base text-danger">
          <AlertTriangle className="size-4" /> {q.error.message}
        </div>
      )}
      {q.data?.probes.some((p) => p.configured === false) && (
        <p className="mb-3.5 text-sm text-fg-muted" data-testid="probes-not-set-up">{t("services.notSetUp")}</p>
      )}
      {q.data?.probes.length === 0 && (
        <div data-testid="services-empty">
          <EmptyState illustration={<Activity className="size-10 text-fg-subtle" />} title={t("services.empty.title")} description={t("services.empty.body")}
            action={canConfigure ? <LinkButton size="sm" href="/admin/platform" tooltip={t("services.empty.actionTooltip")}>{t("services.empty.action")}</LinkButton> : undefined} />
        </div>
      )}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {q.isPending &&
          Array.from({ length: 4 }, (_, i) => <Skeleton key={i} className="h-49 rounded-xl" />)}
        {q.data?.probes.map((p) => (
          <ServiceCard key={p.name} probe={p} instance={q.data.instances.find((i) => i.name === p.instance)} />
        ))}
      </div>
    </section>
  );
}
