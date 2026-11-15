import { useState } from "react";
import { AlertTriangle, Boxes, Cpu, Layers, Plus, Server } from "lucide-react";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { DataTable, type ColumnDef, type TableSort } from "../../../lib/design/components/data-table/data-table";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { Meter } from "../../../lib/design/components/meter/meter";
import { StatusDot } from "../../../lib/design/components/status-dot/status-dot";
import { formatBytes } from "../../../lib/format";
import type { InfraOverview } from "../../../lib/hooks/use-infra";
import { useNavigate } from "../../../lib/router/router";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { StatTile } from "../../overview/ui/stat-tile";
import { fleetRows, hostHref, providersWithHosts, sortFleet, type FleetRow, type FleetSortKey } from "../model";
import { ActivityCard } from "./activity-card";
import { ProviderCards } from "./provider-cards";
import { SystemIdentityForm } from "./system-identity-form";

type Props = { data: InfraOverview; onAdd?: (kind?: string, existing?: string) => void };

export function OverviewTab({ data, onAdd }: Props) {
  const [editingSystem, setEditingSystem] = useState<string | null>(null);
  const t = useText();
  const nav = useNavigate();
  const [sort, setSort] = useState<TableSort>({ column: "name", direction: "ascending" });
  const all = fleetRows(data);
  const rows = sortFleet(all, sort.column as FleetSortKey, sort.direction);
  const memoryRows = sortFleet(all.filter((r) => r.memFraction !== null), "memory", "descending");
  const providerGuestRows = data.providers.filter((p) => p.nodes.length > 0);
  const inUse = providersWithHosts(data).length;
  const notReported = <span className="text-fg-subtle">{t("infra.fleet.notReported")}</span>;
  const columns: ColumnDef<FleetRow>[] = [
    {
      id: "name", label: t("infra.column.name"), isRowHeader: true, sortable: true,
      render: (r) => (
        <>
          <span className="font-medium text-fg">{r.node.label}</span>
          {r.node.control_plane && <Badge tone="warning" className="ml-2">{t("infra.controlPlane")}</Badge>}
          {r.node.source === "agent" && <span className="ml-2 text-xs text-fg-subtle">{t("infra.status.localAgent")}</span>}
          {(r.node.version || r.node.details?.cluster || r.node.base_url || r.node.scope) && (
            <p className="mt-0.5 max-w-xs truncate font-mono text-xs text-fg-subtle">{[r.node.version, r.node.details?.cluster, r.node.base_url ?? r.node.scope].filter(Boolean).join(" · ")}</p>
          )}
        </>
      ),
    },
    { id: "provider", label: t("infra.column.provider"), sortable: true, render: (r) => <Badge tone="neutral">{r.providerLabel}</Badge> },
    {
      id: "state", label: t("infra.column.status"), sortable: true,
      render: (r) => (
        <>
          <Badge tone={r.health === "operational" ? "success" : r.health === "down" ? "danger" : "neutral"}>
            <StatusDot health={r.health === "operational" ? "up" : r.health === "down" ? "down" : "unknown"} />
            {t(`infra.health.${r.health}`)}
          </Badge>
          {(r.node.health_checked_at || r.node.health_latency_ms !== undefined) && <p className="mt-1 text-xs text-fg-subtle">
            {r.node.health_checked_at && <span>{t("infra.health.checked_at", { time: new Date(r.node.health_checked_at).toLocaleString() })}</span>}
            {r.node.health_latency_ms !== undefined && <span className="ml-2">{t("infra.health.latency", { latency: r.node.health_latency_ms })}</span>}
          </p>}
          {r.node.error && <p className="mt-1 max-w-xs truncate text-xs text-danger" title={r.node.error}>{r.node.error}</p>}
        </>
      ),
    },
    { id: "cpu", label: t("infra.column.cpu"), sortable: true, render: (r) => (r.cpus === null ? notReported : <span className="tabular-nums">{t("infra.fleet.cpus", { count: r.cpus })}</span>) },
    {
      id: "memory", label: t("infra.column.memory"), sortable: true, width: "12rem",
      render: (r) => (r.memFraction === null ? notReported
        : <Meter label={t("infra.fleet.memoryOf", { label: r.node.label })} used={r.memUsed} total={r.memTotal} text={`${formatBytes(r.memUsed ?? 0)} / ${formatBytes(r.memTotal ?? 0)}`} className="min-w-36" />),
    },
    { id: "disk", label: t("infra.column.disk"), sortable: true, render: (r) => (r.diskBytes === null ? notReported : <span className="tabular-nums">{t("infra.fleet.disk", { size: formatBytes(r.diskBytes), count: r.diskCount })}</span>) },
    {
      id: "guests", label: t("infra.column.guests"), sortable: true,
      render: (r) => <Meter label={t("infra.fleet.guestsOf", { label: r.node.label })} used={r.guestsRunning} total={r.guestsTotal} text={`${r.guestsRunning} / ${r.guestsTotal}`} tone="success" pressure={false} className="min-w-24" />,
    },
    { id: "site", label: t("infra.column.site"), sortable: true, render: (r) => r.node.site ?? "" },
    { id: "fleet", label: t("infra.column.fleet"), sortable: true, render: (r) => r.fleetLabel },
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
        <p className="mb-6 rounded-lg border border-line bg-raised/60 px-3 py-2 text-sm text-fg-muted" role="status">
          {t("infra.agent.down", { label: data.local_agent.label })}
        </p>
      )}
      <div className="mb-6 grid gap-4 lg:grid-cols-2">
        <Card className="overflow-hidden">
          <CardHeader title={t("infra.graph.memory.title")} description={t("infra.graph.memory.description")} />
          {memoryRows.length === 0 ? (
            <p className="p-5 text-base text-fg-muted">{t("infra.graph.memory.empty")}</p>
          ) : (
            <ul className="space-y-3 p-5">
              {memoryRows.map((r) => (
                <li key={r.id}>
                  <Meter label={`${r.node.label} (${r.providerLabel})`} used={r.memUsed} total={r.memTotal} text={`${formatBytes(r.memUsed ?? 0)} / ${formatBytes(r.memTotal ?? 0)}`} size="md" />
                </li>
              ))}
            </ul>
          )}
        </Card>
        <Card className="overflow-hidden">
          <CardHeader title={t("infra.graph.guests.title")} description={t("infra.graph.guests.description")} />
          {providerGuestRows.length === 0 ? (
            <p className="p-5 text-base text-fg-muted">{t("infra.graph.guests.empty")}</p>
          ) : (
            <ul className="space-y-3 p-5">
              {providerGuestRows.map((provider) => (
                <li key={provider.kind}>
                  <Meter label={`${provider.label} (${provider.kind})`} used={provider.totals.running} total={provider.totals.guests}
                    text={`${provider.totals.running} / ${provider.totals.guests}`} tone="success" pressure={false} size="md" />
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>
      <div className="mb-6"><ProviderCards providers={data.providers} onAdd={onAdd && ((kind) => onAdd(kind))} /></div>
      <Card elevation={2} className="mb-6 overflow-hidden">
        <CardHeader title={t("infra.nodes.title")} description={t("infra.nodes.description")} />
        <div className="overflow-x-auto"><DataTable
          label={t("infra.nodes.title")}
          columns={columns}
          rows={rows}
          sort={sort}
          onSortChange={setSort}
          dependencies={[t]}
          onRowAction={(id) => { const r = all.find((x) => x.id === id); if (r) nav(hostHref(r.kind, r.node.id)); }}
          empty={
            <EmptyState
              illustration={<Server className="size-10 text-fg-subtle" />}
              title={t("infra.nodes.empty.title")}
              description={t("infra.nodes.empty.body")}
              action={onAdd && <Button variant="primary" size="sm" tooltip={t("infra.add.tooltip")} onPress={() => onAdd?.()}><Plus className="size-4" /> {t("infra.add")}</Button>}
            />
          }
        /></div>
      </Card>
      <div className="mb-6"><ActivityCard showHost /></div>
      {data.unattached.length > 0 && (
        <Card className="overflow-hidden">
          <CardHeader title={t("infra.unattached.title")} description={t("infra.unattached.description")} />
          <ul className="divide-y divide-line">
            {data.unattached.map((s) => (
              <li key={s.id} className="px-5 py-3 text-base">
                <div className="flex items-center justify-between gap-3">
                  <span><span className="font-medium text-fg">{s.label || s.slug}</span> <span className="font-mono text-sm text-fg-subtle">{s.slug}</span></span>
                  <div className="flex gap-2">
                    {onAdd && <Button size="sm" variant="secondary" tooltip={t("infra.unattached.attachTooltip", { label: s.label || s.slug })} onPress={() => onAdd(undefined, s.id)}>
                      {t("infra.unattached.attach")}
                    </Button>}
                    {onAdd && <Button size="sm" variant="ghost" tooltip={editingSystem === s.id ? "Close system identity editor" : `Edit identity for ${s.label || s.slug}`} onPress={() => setEditingSystem(editingSystem === s.id ? null : s.id)}>
                      {editingSystem === s.id ? "Close identity" : "Edit identity"}
                    </Button>}
                  </div>
                </div>
                {editingSystem === s.id && <div className="mt-4"><SystemIdentityForm serverId={s.id} /></div>}
              </li>
            ))}
          </ul>
        </Card>
      )}
    </>
  );
}
