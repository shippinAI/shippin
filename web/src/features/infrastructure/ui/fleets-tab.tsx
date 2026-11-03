import { useState } from "react";
import { Layers, Plus } from "lucide-react";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { TextField } from "../../../lib/design/components/field/text-field";
import { StatusDot } from "../../../lib/design/components/status-dot/status-dot";
import { useDeleteFleet, useSaveFleet, useSetFleet, type InfraOverview } from "../../../lib/hooks/use-infra";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { fleetMembers, fleetTotals, hostsWithoutFleet } from "../model";

function FleetCard({ data, fleetId }: { data: InfraOverview; fleetId: string }) {
  const t = useText();
  const fleet = data.fleets.find((f) => f.id === fleetId)!;
  const members = fleetMembers(data, fleetId);
  const totals = fleetTotals(members);
  const free = hostsWithoutFleet(data);
  const assign = useSetFleet();
  const remove = useDeleteFleet();
  return (
    <Card className="mb-6 overflow-hidden" data-testid="fleet">
      <CardHeader title={fleet.label} description={[fleet.description, t("infra.fleets.summary", { online: totals.online, nodes: totals.nodes, running: totals.running, guests: totals.guests })].filter(Boolean).join(" · ")}
        actions={<Button size="sm" variant="ghost" tooltip={t("infra.fleets.deleteTooltip")} isDisabled={remove.isPending} onPress={() => remove.mutate(fleetId)}>{t("infra.fleets.delete")}</Button>} />
      {members.length === 0 ? <p className="p-5 text-sm text-fg-muted">{t("infra.fleets.noHosts")}</p> : (
        <ul className="divide-y divide-line">
          {members.map(({ node, providerLabel }) => (
            <li key={node.id} className="flex flex-wrap items-center gap-3 px-5 py-3 text-sm" data-testid="fleet-member">
              <span className="font-medium text-fg">{node.label}</span>
              <Badge tone="primary">{providerLabel}</Badge>
              {node.version && <span className="font-mono text-[12.5px] text-fg-muted">{node.version}</span>}
              {node.details?.cluster && <span className="text-[12.5px] text-fg-muted">{t("infra.fact.clusterNamed", { name: node.details.cluster })}</span>}
              <Badge tone={node.online ? "success" : "danger"}><StatusDot health={node.online ? "up" : "down"} /> {node.online ? t("infra.status.online") : t("infra.status.offline")}</Badge>
              <span className="tabular-nums text-fg-muted">{t("infra.fleets.memberGuests", { running: node.guests.running, total: node.guests.total })}</span>
              <Button size="sm" variant="ghost" className="ml-auto" tooltip={t("infra.fleets.removeTooltip")} isDisabled={assign.isPending} onPress={() => assign.mutate({ serverId: node.id, fleetId: "" })}>{t("infra.fleets.remove")}</Button>
            </li>
          ))}
        </ul>
      )}
      {free.length > 0 && (
        <div className="border-t border-line px-5 py-3">
          <label className="flex flex-wrap items-center gap-2 text-[13px] text-fg-muted">
            {t("infra.fleets.addHost")}
            <select aria-label={t("infra.fleets.addHost")} className="h-8 rounded-md border border-line bg-canvas px-2 text-sm" value="" disabled={assign.isPending}
              onChange={(e) => { if (e.target.value) assign.mutate({ serverId: e.target.value, fleetId }); }}>
              <option value="">{t("infra.fleets.chooseHost")}</option>
              {free.map(({ node, providerLabel }) => <option key={node.id} value={node.id}>{node.label} · {providerLabel}</option>)}
            </select>
          </label>
        </div>
      )}
    </Card>
  );
}

/** Group hosts of any provider. A fleet lives in this panel: each host keeps its own provider and cluster. */
export function FleetsTab({ data }: { data: InfraOverview }) {
  const t = useText();
  const save = useSaveFleet();
  const [label, setLabel] = useState("");
  const [description, setDescription] = useState("");
  const create = () => save.mutate({ label: label.trim(), description: description.trim() }, { onSuccess: () => { setLabel(""); setDescription(""); } });
  return (
    <>
      <p className="mb-6 max-w-3xl text-sm text-fg-muted">{t("infra.fleets.explain")}</p>
      {data.fleets.length === 0 && (
        <div className="mb-6"><EmptyState illustration={<Layers className="size-10 text-fg-subtle" />} title={t("infra.fleets.empty.title")} description={t("infra.fleets.empty.body")} /></div>
      )}
      {data.fleets.map((f) => <FleetCard key={f.id} data={data} fleetId={f.id} />)}
      <Card className="p-5">
        <h2 className="mb-3 text-sm font-semibold text-fg">{t("infra.fleets.new")}</h2>
        <form className="grid items-start gap-3 md:grid-cols-[1fr_2fr_auto]" onSubmit={(e) => { e.preventDefault(); if (label.trim()) create(); }}>
          <TextField label={t("infra.fleets.name")} value={label} onChange={setLabel} />
          <TextField label={t("infra.fleets.description")} value={description} onChange={setDescription} />
          <div className="md:pt-6">
            <Button type="submit" variant="primary" size="sm" tooltip={t("infra.fleets.createTooltip")} isDisabled={!label.trim() || save.isPending}><Plus className="size-4" /> {t("infra.fleets.create")}</Button>
          </div>
        </form>
      </Card>
    </>
  );
}
