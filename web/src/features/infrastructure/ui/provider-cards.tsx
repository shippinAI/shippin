import { Button } from "../../../lib/design/components/button/button";
import { LinkButton } from "../../../lib/design/components/button/link-button";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { StatusDot } from "../../../lib/design/components/status-dot/status-dot";
import type { InfraProvider } from "../../../lib/hooks/use-infra";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { OVERVIEW_TAB, tabHref } from "../model";

type Props = { providers: InfraProvider[]; onAdd?: (kind: string) => void };

/** One card per provider this installation offers: what is added, whether it answers, and how to add it. */
export function ProviderCards({ providers, onAdd }: Props) {
  const t = useText();
  return (
    <Card className="overflow-hidden">
      <CardHeader title={t("infra.providers.title")} description={t("infra.providers.description")} />
      <ul className="grid gap-px bg-line sm:grid-cols-2" data-testid="provider-cards">
        {providers.map((p) => {
          const added = p.nodes.length > 0;
          const healthy = added && p.totals.online === p.totals.nodes;
          return (
            <li key={p.kind} className="flex flex-col gap-3 bg-surface p-5" data-testid={`provider-${p.kind}`}>
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <h3 className="text-sm font-semibold text-fg">{p.label}</h3>
                  {p.description && <p className="mt-1 text-[13px] text-fg-muted">{p.description}</p>}
                </div>
                <Badge tone={!added ? "neutral" : healthy ? "success" : "warning"}>
                  {added && <StatusDot health={healthy ? "up" : "unknown"} />} {added ? t("infra.provider.added") : t("infra.provider.notAdded")}
                </Badge>
              </div>
              <p className="flex-1 text-[13px] text-fg-subtle">
                {added
                  ? t("infra.provider.hosts", { online: p.totals.online, total: p.totals.nodes, running: p.totals.running, guests: p.totals.guests })
                  : t("infra.provider.none")}
              </p>
              <div>
                {added ? (
                  <LinkButton size="sm" href={tabHref(p.kind)} tooltip={t("infra.provider.openTooltip", { label: p.label })}>{t("infra.provider.open")}</LinkButton>
                ) : !p.attachable ? null : onAdd ? (
                  <Button size="sm" variant="primary" tooltip={t("infra.provider.addTooltip", { label: p.label })} onPress={() => onAdd(p.kind)}>
                    {t("infra.provider.add", { label: p.label })}
                  </Button>
                ) : (
                  <LinkButton size="sm" href={tabHref(OVERVIEW_TAB)} tooltip={t("infra.provider.addTooltip", { label: p.label })}>
                    {t("infra.provider.add", { label: p.label })}
                  </LinkButton>
                )}
              </div>
            </li>
          );
        })}
      </ul>
    </Card>
  );
}
