import { useState } from "react";
import { AlertTriangle, Boxes, Cpu, Layers, Plus, Server } from "lucide-react";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { DataTable, type ColumnDef } from "../../../lib/design/components/data-table/data-table";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { StatusDot } from "../../../lib/design/components/status-dot/status-dot";
import { formatBytes } from "../../../lib/format";
import type { InfraNode, InfraOverview } from "../../../lib/hooks/use-infra";
import { useNavigate } from "../../../lib/router/router";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { StatTile } from "../../overview/ui/stat-tile";
import { memoryPercent, providersWithHosts, tabHref } from "../model";
import { ActivityCard } from "./activity-card";
import { ProviderCards } from "./provider-cards";
import { SystemIdentityForm } from "./system-identity-form";

type Row = InfraNode & { providerLabel: string };
type Props = { data: InfraOverview; onAdd?: (kind?: string, existing?: string) => void };

export function OverviewTab({ data, onAdd }: Props) {
  const [editingSystem, setEditingSystem] = useState<string | null>(null);
  const t = useText();
  const nav = useNavigate();
  const rows: Row[] = data.providers.flatMap((p) => p.nodes.map((n) => ({ ...n, id: `${p.kind}:${n.id}`, providerLabel: p.label })));
  const memoryRows = rows
    .filter((n) => n.details?.mem_total_bytes)
    .map((n) => ({ node: n, percent: memoryPercent(n.details) ?? 0 }))
    .sort((a, b) => b.percent - a.percent);
  const providerGuestRows = data.providers.filter((p) => p.nodes.length > 0);
  const inUse = providersWithHosts(data).length;
  const columns: ColumnDef<Row>[] = [
    {
      id: "name", label: t("infra.column.name"), isRowHeader: true,
      render: (n) => (
        <>
          <span className="font-medium text-fg">{n.label}</span>
          <Badge tone="neutral" className="ml-2">{n.kind}</Badge>
          {n.control_plane && <Badge tone="warning" className="ml-2">{t("infra.controlPlane")}</Badge>}
          {n.source === "agent" && <span className="ml-2 text-xs text-fg-subtle">{t("infra.status.localAgent")}</span>}
        </>
      ),
    },
    { id: "provider", label: t("infra.column.provider"), render: (n) => n.providerLabel },
    { id: "site", label: t("infra.column.site"), render: (n) => n.site ?? "" },
    {
      id: "status", label: t("infra.column.status"),
      render: (n) => (
        <>
          <Badge tone={n.health_state === "operational" ? "success" : n.health_state === "down" ? "danger" : "neutral"}>
            <StatusDot health={n.health_state === "operational" ? "up" : n.health_state === "down" ? "down" : "unknown"} />
            {t(`infra.health.${n.health_state}`)}
          </Badge>
          {(n.health_checked_at || n.health_latency_ms !== undefined) && <p className="mt-1 text-xs text-fg-subtle">
            {n.health_checked_at && <span>{t("infra.health.checked_at", { time: new Date(n.health_checked_at).toLocaleString() })}</span>}
            {n.health_latency_ms !== undefined && <span className="ml-2">{t("infra.health.latency", { latency: n.health_latency_ms })}</span>}
          </p>}
          {n.error && <p className="mt-1 max-w-xs truncate text-xs text-danger" title={n.error}>{n.error}</p>}
        </>
      ),
    },
    { id: "version", label: t("infra.column.version"), render: (n) => <span className="font-mono text-sm">{n.version ?? ""}</span> },
    { id: "cluster", label: t("infra.column.cluster"), render: (n) => n.details?.cluster ?? "" },
    { id: "fleet", label: t("infra.column.fleet"), render: (n) => data.fleets.find((f) => f.id === n.fleet_id)?.label ?? "" },
    { id: "guests", label: t("infra.column.guests"), render: (n) => <span className="tabular-nums">{n.guests.running} / {n.guests.total}</span> },
    { id: "address", label: t("infra.column.address"), render: (n) => <span className="font-mono text-sm">{n.base_url ?? n.scope ?? ""}</span> },
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
            <p className="px-5 pb-5 text-base text-fg-muted">{t("infra.graph.memory.empty")}</p>
          ) : (
            <ul className="space-y-3 px-5 pb-5">
              {memoryRows.map(({ node, percent }) => (
                <li key={node.id}>
                  <div className="mb-1 flex items-center justify-between gap-3 text-xs">
                    <span className="truncate text-fg"><span className="font-medium">{node.label}</span> <span className="text-fg-subtle">({node.kind})</span></span>
                    <span className="tabular-nums text-fg-subtle">
                      {formatBytes(node.details?.mem_used_bytes ?? 0)} / {formatBytes(node.details?.mem_total_bytes ?? 0)}
                    </span>
                  </div>
                  <div className="h-2 overflow-hidden rounded-full bg-raised">
                    <div className="h-full rounded-full bg-primary" style={{ width: `${percent}%` }} />
                  </div>
                </li>
              ))}
            </ul>
          )}
        </Card>
        <Card className="overflow-hidden">
          <CardHeader title={t("infra.graph.guests.title")} description={t("infra.graph.guests.description")} />
          {providerGuestRows.length === 0 ? (
            <p className="px-5 pb-5 text-base text-fg-muted">{t("infra.graph.guests.empty")}</p>
          ) : (
            <ul className="space-y-3 px-5 pb-5">
              {providerGuestRows.map((provider) => {
                const total = provider.totals.guests;
                const running = provider.totals.running;
                const percent = total > 0 ? Math.round((running / total) * 100) : 0;
                return (
                  <li key={provider.kind}>
                    <div className="mb-1 flex items-center justify-between gap-3 text-xs">
                      <span className="truncate text-fg"><span className="font-medium">{provider.label}</span> <span className="text-fg-subtle">({provider.kind})</span></span>
                      <span className="tabular-nums text-fg-subtle">{running} / {total}</span>
                    </div>
                    <div className="h-2 overflow-hidden rounded-full bg-raised">
                      <div className="h-full rounded-full bg-success" style={{ width: `${percent}%` }} />
                    </div>
                  </li>
                );
              })}
            </ul>
          )}
        </Card>
      </div>
      <div className="mb-6"><ProviderCards providers={data.providers} onAdd={onAdd && ((kind) => onAdd(kind))} /></div>
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
              action={onAdd && <Button variant="primary" size="sm" tooltip={t("infra.add.tooltip")} onPress={() => onAdd?.()}><Plus className="size-4" /> {t("infra.add")}</Button>}
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
