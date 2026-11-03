import { AlertTriangle, Boxes, Cpu, Layers, Plus, Server } from "lucide-react";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { DataTable, type ColumnDef } from "../../../lib/design/components/data-table/data-table";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { StatusDot } from "../../../lib/design/components/status-dot/status-dot";
import type { InfraNode, InfraOverview } from "../../../lib/hooks/use-infra";
import { useNavigate } from "../../../lib/router/router";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { StatTile } from "../../overview/ui/stat-tile";
import { providersWithHosts, tabHref } from "../model";
import { ActivityCard } from "./activity-card";
import { ProviderCards } from "./provider-cards";

type Row = InfraNode & { providerLabel: string };
type Props = { data: InfraOverview; onAdd: (kind?: string, existing?: string) => void };

export function OverviewTab({ data, onAdd }: Props) {
  const t = useText();
  const nav = useNavigate();
  const rows: Row[] = data.providers.flatMap((p) => p.nodes.map((n) => ({ ...n, id: `${p.kind}:${n.id}`, providerLabel: p.label })));
  const inUse = providersWithHosts(data).length;
  const columns: ColumnDef<Row>[] = [
    {
      id: "name", label: t("infra.column.name"), isRowHeader: true,
      render: (n) => (
        <>
          <span className="font-medium text-fg">{n.label}</span>
          {n.source === "agent" && <span className="ml-2 text-[12px] text-fg-subtle">{t("infra.status.localAgent")}</span>}
        </>
      ),
    },
    { id: "provider", label: t("infra.column.provider"), render: (n) => n.providerLabel },
    { id: "site", label: t("infra.column.site"), render: (n) => n.site ?? "" },
    {
      id: "status", label: t("infra.column.status"),
      render: (n) => (
        <>
          <Badge tone={n.online && !n.error ? "success" : "danger"}>
            <StatusDot health={n.online && !n.error ? "up" : "down"} /> {n.online ? t("infra.status.online") : t("infra.status.offline")}
          </Badge>
          {n.error && <p className="mt-1 max-w-xs truncate text-[12px] text-danger" title={n.error}>{n.error}</p>}
        </>
      ),
    },
    { id: "version", label: t("infra.column.version"), render: (n) => <span className="font-mono text-[12.5px]">{n.version ?? ""}</span> },
    { id: "cluster", label: t("infra.column.cluster"), render: (n) => n.details?.cluster ?? "" },
    { id: "fleet", label: t("infra.column.fleet"), render: (n) => data.fleets.find((f) => f.id === n.fleet_id)?.label ?? "" },
    { id: "guests", label: t("infra.column.guests"), render: (n) => <span className="tabular-nums">{n.guests.running} / {n.guests.total}</span> },
    { id: "address", label: t("infra.column.address"), render: (n) => <span className="font-mono text-[12.5px]">{n.base_url ?? n.scope ?? ""}</span> },
  ];
  return (
    <>
      <div className="mb-8 grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatTile label={t("infra.stat.providers")} value={`${inUse}/${data.providers.length}`} hint={t("infra.stat.providers.hint")} icon={Layers} hue="var(--color-primary)" />
        <StatTile label={t("infra.stat.nodes")} value={`${data.totals.online}/${data.totals.nodes}`} hint={t("infra.stat.nodes.hint")} icon={Cpu} hue="var(--color-success)" />
        <StatTile label={t("infra.stat.guests")} value={`${data.totals.running}/${data.totals.guests}`} hint={t("infra.stat.guests.hint")} icon={Boxes} hue="var(--color-hue-workspaces)" />
        <StatTile label={t("infra.stat.attention")} value={data.totals.nodes - data.totals.online} hint={t("infra.stat.attention.hint")} icon={AlertTriangle} hue="var(--color-warning)" />
      </div>
      {data.local_agent && (
        <p className="mb-6 rounded-lg border border-line bg-raised/60 px-3 py-2 text-[13px] text-fg-muted" role="status">
          {t("infra.agent.down", { label: data.local_agent.label })}
        </p>
      )}
      <div className="mb-6"><ProviderCards providers={data.providers} onAdd={(kind) => onAdd(kind)} /></div>
      <Card elevation={2} className="mb-6 overflow-hidden">
        <CardHeader title={t("infra.nodes.title")} description={t("infra.nodes.description")} />
        <DataTable
          label={t("infra.nodes.title")}
          columns={columns}
          rows={rows}
          onRowAction={(id) => nav(tabHref(id.split(":")[0]!))}
          empty={
            <EmptyState
              illustration={<Server className="size-10 text-fg-subtle" />}
              title={t("infra.nodes.empty.title")}
              description={t("infra.nodes.empty.body")}
              action={<Button variant="primary" size="sm" tooltip={t("infra.add.tooltip")} onPress={() => onAdd()}><Plus className="size-4" /> {t("infra.add")}</Button>}
            />
          }
        />
      </Card>
      <div className="mb-6"><ActivityCard showHost /></div>
      {data.unattached.length > 0 && (
        <Card className="overflow-hidden">
          <CardHeader title={t("infra.unattached.title")} description={t("infra.unattached.description")} />
          <ul className="divide-y divide-line">
            {data.unattached.map((s) => (
              <li key={s.id} className="flex items-center justify-between gap-3 px-5 py-3 text-sm">
                <span><span className="font-medium text-fg">{s.label || s.slug}</span> <span className="font-mono text-[12.5px] text-fg-subtle">{s.slug}</span></span>
                <Button size="sm" variant="secondary" tooltip={t("infra.unattached.attachTooltip", { label: s.label || s.slug })} onPress={() => onAdd(undefined, s.id)}>
                  {t("infra.unattached.attach")}
                </Button>
              </li>
            ))}
          </ul>
        </Card>
      )}
    </>
  );
}
