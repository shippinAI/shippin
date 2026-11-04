import { useEffect, useState } from "react";
import { Cpu } from "lucide-react";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { DataTable, type ColumnDef } from "../../../lib/design/components/data-table/data-table";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { TextField } from "../../../lib/design/components/field/text-field";
import { Select } from "../../../lib/design/components/select/select";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import { StatusDot } from "../../../lib/design/components/status-dot/status-dot";
import { formatBytes } from "../../../lib/format";
import { isWaitingOnBoard } from "../../../lib/api/client";
import { useServices } from "../../../lib/hooks/use-api";
import { useCreateGuest, useDeleteGuest, useGuestAction, useGuests, useRollback, useSnapshot, useSnapshots, useTask, type Guest } from "../../../lib/hooks/use-hosts";
import { isOpen as operationOpen, useDetachHost, useInfraCan, useOperations, useSaveSystemIdentity, useSystemIdentity, type InfraNode, type InfraProvider } from "../../../lib/hooks/use-infra";

import { useText } from "../../../lib/ui-config/use-ui-config";
import { firstAnswering, formatUptime, isRunning, isStopped, stoppableGuests, verbApplies } from "../model";
import { ActivityCard } from "./activity-card";
import { BoardNotice } from "./board-notice";
import { ChangeNoteDialog, type PendingChange } from "./change-note-dialog";
import { CreateGuestCard } from "./create-guest-card";
import { NodeFacts } from "./node-details";
import { StopGuestsDialog } from "./stop-guests-dialog";

function useGuestColumns(): ColumnDef<Guest>[] {
  const t = useText();
  return [
    {
      id: "name", label: t("infra.guest.name"), isRowHeader: true,
      render: (g) => (
        <>
          <span className="font-medium text-fg">{g.name || g.id}</span>
          {g.self && <Badge tone="warning" className="ml-2">{t("infra.guest.self")}</Badge>}
        </>
      ),
    },
    { id: "id", label: t("infra.guest.id"), render: (g) => <span className="font-mono text-[12.5px]">{g.id}</span> },
    { id: "kind", label: t("infra.guest.kind"), render: (g) => (g.kind === "ct" ? t("infra.guest.ct") : t("infra.guest.vm")) },
    {
      id: "status", label: t("infra.guest.status"),
      render: (g) => (
        <Badge tone={isRunning(g.status) ? "success" : isStopped(g.status) ? "neutral" : "warning"}>
          <StatusDot health={isRunning(g.status) ? "up" : "unknown"} /> {g.status}
        </Badge>
      ),
    },
    { id: "node", label: t("infra.guest.node"), render: (g) => g.node },
  ];
}

/** The local agent's project. It is read-only: changes go through the agent's own job flow. */
function AgentGuests({ node }: { node: InfraNode }) {
  const t = useText();
  const q = useServices();
  const columns = useGuestColumns();
  const rows: Guest[] = (q.data?.instances ?? []).map((i) => ({
    id: i.name, name: i.name, kind: i.type === "virtual-machine" ? "vm" : "ct", status: i.status, node: node.label,
  }));
  return (
    <Card elevation={2} className="overflow-hidden">
      <CardHeader title={node.label} description={t("infra.provider.readOnly")} />
      {q.isPending ? (
        <div className="space-y-3 p-5">{Array.from({ length: 4 }, (_, i) => <Skeleton key={i} className="h-9" />)}</div>
      ) : (
        <DataTable label={node.label} columns={columns} rows={rows}
          empty={<EmptyState illustration={<Cpu className="size-10 text-fg-subtle" />} title={t("infra.provider.noGuests.title")} description={q.error?.message ?? t("infra.provider.noGuests.body")} />} />
      )}
    </Card>
  );
}

function ApiGuests({ provider, node }: { provider: InfraProvider; node: InfraNode }) {
  const t = useText();
  const can = (c: string) => provider.capabilities.includes(c);
  const base = `/v1/infra/servers/${node.id}`;
  const columns = useGuestColumns();
  const guests = useGuests(base);
  const [guestId, setGuestId] = useState("");
  const guest = (guests.data?.items ?? []).find((g) => g.id === guestId);
  const snaps = useSnapshots(can("snapshot_list") ? base : "", guest?.id ?? "");
  const act = useGuestAction(base);
  const operations = useOperations(node.id);
  const snap = useSnapshot(base);
  const back = useRollback(base);
  const create = useCreateGuest(node.id);
  const remove = useDeleteGuest(base);
  const detach = useDetachHost();
  const may = useInfraCan();
  const [change, setChange] = useState<PendingChange | null>(null);
  const [stopping, setStopping] = useState(false);
  const [snapName, setSnapName] = useState("");
  const [upid, setUpid] = useState("");
  const [lookup, setLookup] = useState("");
  const task = useTask(can("tasks") ? base : "", lookup);
  const waiting = [
    { error: act.error, retry: () => { if (act.variables) act.mutate(act.variables); } },
    { error: snap.error, retry: () => { if (snap.variables) snap.mutate(snap.variables); } },
    { error: back.error, retry: () => { if (back.variables) back.mutate(back.variables); } },
    { error: create.error, retry: () => { if (create.variables) create.mutate(create.variables); } },
    { error: remove.error, retry: () => { if (remove.variables) remove.mutate(remove.variables); } },
  ].find((m) => isWaitingOnBoard(m.error));
  // One slow change holds its own guest, and only that one.
  const guestBusy = !!guest && ((act.isPending && act.variables?.guest === guest.id) || (operations.data?.items ?? []).some((o) => operationOpen(o) && o.guest === guest.id));
  return (
    <>
      {waiting && <div className="mb-6"><BoardNotice error={waiting.error} retry={waiting.retry} /></div>}
      <Card elevation={2} className="mb-6 overflow-hidden">
        <CardHeader
          title={node.label}
          description={[node.control_plane ? t("infra.controlPlane") : "", node.base_url, node.scope, node.version].filter(Boolean).join(" · ")}
          actions={
            <div className="flex gap-2">
              {can("lifecycle") && may("infra.guests.power") && (
                <Button size="sm" variant="secondary" tooltip={t("infra.stop.tooltip")} isDisabled={stoppableGuests(guests.data?.items ?? []).length === 0} onPress={() => setStopping(true)}>{t("infra.stop.open")}</Button>
              )}
              {may("infra.hosts.write") && (
                <Button size="sm" variant="ghost" tooltip={t("infra.detach.tooltip")}
                  onPress={() => setChange({
                    title: t("infra.detach.title", { label: node.label }), description: t("infra.detach.body"),
                    warning: node.control_plane ? t("infra.detach.controlPlane") : undefined, confirm: t("infra.detach.confirm"),
                    run: (note) => detach.mutate({ id: node.id, label: node.label, note }),
                  })}>{t("infra.detach")}</Button>
              )}
            </div>
          }
        />
        {guests.isPending && node.online ? (
          <div className="space-y-3 p-5">{Array.from({ length: 4 }, (_, i) => <Skeleton key={i} className="h-9" />)}</div>
        ) : (
          <DataTable label={node.label} columns={columns} rows={guests.data?.items ?? []} onRowAction={setGuestId}
            empty={
              <EmptyState illustration={<Cpu className="size-10 text-fg-subtle" />}
                title={guests.isError || !node.online ? t("infra.provider.empty.title") : t("infra.provider.noGuests.title")}
                description={guests.error?.message ?? node.error ?? t("infra.provider.noGuests.body")} />
            } />
        )}
      </Card>
      {guest && (can("lifecycle") || can("snapshot") || can("snapshot_list")) && (
        <Card className="mb-6 p-5">
          <h2 className="mb-3 text-sm font-semibold text-fg">{guest.name || guest.id}</h2>
          {guest.metrics && (
            <dl className="mb-4 grid gap-3 rounded-lg border border-line bg-canvas/40 p-3 text-xs sm:grid-cols-2 lg:grid-cols-4" data-testid="guest-metrics">
              <div><dt className="text-fg-subtle">CPU</dt><dd className="mt-1 font-medium text-fg">{(guest.metrics.cpu_fraction * 100).toFixed(1)}%{guest.metrics.cpu_count ? ` · ${guest.metrics.cpu_count} vCPU` : ""}</dd></div>
              <div><dt className="text-fg-subtle">Memory</dt><dd className="mt-1 font-medium text-fg">{formatBytes(guest.metrics.memory_used_bytes)} / {formatBytes(guest.metrics.memory_total_bytes)}</dd></div>
              <div><dt className="text-fg-subtle">Disk</dt><dd className="mt-1 font-medium text-fg">{formatBytes(guest.metrics.disk_used_bytes)} / {formatBytes(guest.metrics.disk_total_bytes)}</dd></div>
              <div><dt className="text-fg-subtle">Network I/O</dt><dd className="mt-1 font-medium text-fg">{formatBytes(guest.metrics.network_in_bytes)} in · {formatBytes(guest.metrics.network_out_bytes)} out</dd></div>
              {guest.uptime_seconds !== undefined && <div><dt className="text-fg-subtle">Uptime</dt><dd className="mt-1 font-medium text-fg">{formatUptime(guest.uptime_seconds)}</dd></div>}
              {guest.pool && <div><dt className="text-fg-subtle">Pool</dt><dd className="mt-1 font-medium text-fg">{guest.pool}</dd></div>}
              {guest.template && <div><dt className="text-fg-subtle">Template</dt><dd className="mt-1 font-medium text-fg">Yes</dd></div>}
              {guest.tags && <div className="sm:col-span-2"><dt className="text-fg-subtle">Tags</dt><dd className="mt-1 font-medium text-fg">{guest.tags}</dd></div>}
            </dl>
          )}
          {can("lifecycle") && may("infra.guests.power") && (
            <div className="mb-4 flex flex-wrap gap-2">
              {(provider.verbs as ("start" | "stop" | "shutdown" | "reboot")[]).map((verb) => {
                const takesPanelDown = !!guest.self && verb !== "start";
                return (
                  <Button key={verb} size="sm" variant="secondary" tooltip={t("infra.guest.verbTooltip", { verb, guest: guest.id })}
                    isDisabled={guestBusy || !verbApplies(verb, guest.status) || (takesPanelDown && !may("infra.self"))}
                    onPress={() => setChange({
                      title: t("infra.note.title", { change: `${verb} ${guest.name || guest.id}` }),
                      warning: takesPanelDown ? t("infra.self.body") : undefined,
                      confirm: takesPanelDown ? t("infra.self.confirm", { verb }) : verb,
                      run: (note) => act.mutate({ guest: guest.id, verb, note, confirmSelf: takesPanelDown }),
                    })}>
                    {verb}
                  </Button>
                );
              })}
              {can("delete") && may("infra.guests.manage") && (
                <Button size="sm" variant="ghost" tooltip={t("infra.delete.tooltip")} isDisabled={guestBusy || remove.isPending || (!!guest.self && !may("infra.self"))}
                  onPress={() => setChange({
                    title: t("infra.note.title", { change: `${t("infra.delete.action").toLowerCase()} ${guest.name || guest.id}` }),
                    warning: guest.self ? t("infra.self.body") : t("infra.delete.warning"),
                    confirm: t("infra.delete.action"),
                    run: (note) => remove.mutate({ guest: guest.id, note, confirmSelf: !!guest.self }, { onSuccess: () => setGuestId("") }),
                  })}>
                  {t("infra.delete.action")}
                </Button>
              )}
              {guestBusy && <p className="basis-full text-[13px] text-fg-muted" data-testid="guest-busy">{t("infra.guest.busy")}</p>}
            </div>
          )}
          {can("snapshot") && may("infra.guests.snapshot") && (
            <div className="mb-4 flex flex-wrap items-end gap-2">
              <TextField label={t("infra.snapshot.name")} value={snapName} onChange={setSnapName} description={t("infra.snapshot.help")} />
              <Button size="sm" variant="primary" tooltip={t("infra.snapshot.takeTooltip")} isDisabled={!snapName || snap.isPending}
                onPress={() => setChange({
                  title: t("infra.note.title", { change: `${t("infra.snapshot.take").toLowerCase()} ${guest.name || guest.id}` }), confirm: t("infra.snapshot.take"),
                  run: (note) => snap.mutate({ guest: guest.id, snapname: snapName, note }),
                })}>
                {t("infra.snapshot.take")}
              </Button>
            </div>
          )}
          {can("snapshot_list") && (
            <ul className="space-y-2 text-sm">
              {(snaps.data?.items ?? []).map((name) => (
                <li key={name} className="flex items-center justify-between gap-3">
                  <span className="font-mono">{name}</span>
                  {can("rollback") && may("infra.guests.restore") && (
                    <Button size="sm" variant="ghost" tooltip={t("infra.snapshot.rollbackTooltip", { guest: guest.id, name })}
                      isDisabled={back.isPending || guestBusy || (!!guest.self && !may("infra.self"))}
                      onPress={() => setChange({
                        title: t("infra.note.title", { change: `${t("infra.snapshot.rollback").toLowerCase()} ${guest.name || guest.id}` }),
                        warning: guest.self ? t("infra.self.body") : t("infra.snapshot.rollbackWarning", { name }),
                        confirm: t("infra.snapshot.rollback"),
                        run: (note) => back.mutate({ guest: guest.id, name, note, confirmSelf: !!guest.self }),
                      })}>
                      {t("infra.snapshot.rollback")}
                    </Button>
                  )}
                </li>
              ))}
              {snaps.data && snaps.data.items.length === 0 && <li className="text-fg-muted">{t("infra.snapshot.none")}</li>}
            </ul>
          )}
        </Card>
      )}
      {((can("create") && may("infra.guests.manage")) || can("tasks")) && (
        <div className="grid gap-6 lg:grid-cols-2">
          {can("create") && may("infra.guests.manage") && (
            <CreateGuestCard base={base} provider={provider} busy={create.isPending}
              onCreate={(g) => setChange({
                title: t("infra.note.title", { change: `${t("infra.create.submit").toLowerCase()} ${g.name}` }), confirm: t("infra.create.submit"),
                run: (note) => create.mutate({ ...g, note }),
              })} />
          )}
          {can("tasks") && (
            <Card className="p-5">
              <h2 className="mb-3 text-sm font-semibold text-fg">{t("infra.task.title")}</h2>
              <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); setLookup(upid); }}>
                <TextField label={t("infra.task.id")} value={upid} onChange={setUpid} description={t("infra.task.help")} />
                <Button type="submit" size="sm" variant="secondary" tooltip={t("infra.task.lookupTooltip")} isDisabled={!upid}>{t("infra.task.lookup")}</Button>
              </form>
              {task.data && (
                <div className="mt-4 text-sm">
                  <p className="text-fg">{task.data.status}{task.data.exitstatus ? ` · ${task.data.exitstatus}` : ""}{task.data.done ? ` · ${t("infra.task.done")}` : ""}</p>
                  <pre className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap font-mono text-[12px] text-fg-muted">{(task.data.log ?? []).join("\n")}</pre>
                </div>
              )}
              {task.isError && <p className="mt-3 text-sm text-danger">{task.error.message}</p>}
            </Card>
          )}
        </div>
      )}
      <div className="mt-6"><ActivityCard serverId={node.id} /></div>
      <StopGuestsDialog serverId={node.id} guests={guests.data?.items ?? []} isOpen={stopping} onOpenChange={setStopping} />
      <ChangeNoteDialog change={change} onClose={() => setChange(null)} />
    </>
  );
}

export function SystemIdentityForm({ serverId }: { serverId: string }) {
  const identity = useSystemIdentity(serverId);
  const save = useSaveSystemIdentity(serverId);
  const [aliases, setAliases] = useState("");
  const [tags, setTags] = useState("");
  const [label, setLabel] = useState("");
  const [tenantId, setTenantId] = useState("");
  const [machineId, setMachineId] = useState("");
  const [note, setNote] = useState("");
  useEffect(() => {
    if (!identity.data) return;
    setLabel(identity.data.label);
    setAliases(identity.data.aliases.join("\n"));
    setTags(identity.data.tags.join("\n"));
    setTenantId(identity.data.agenthub_tenant_id ?? "");
    setMachineId(identity.data.agenthub_machine_id ?? "");
  }, [identity.data]);
  if (identity.isPending) return <Card className="mb-6 p-5"><p className="text-sm text-fg-muted">Loading system identity…</p></Card>;
  if (identity.isError) return <Card className="mb-6 p-5"><p role="alert" className="text-sm text-danger">{identity.error.message}</p></Card>;
  const split = (value: string) => value.split(/[,\n]/).map((part) => part.trim()).filter(Boolean);
  return (
    <Card elevation={2} className="mb-6 p-5">
      <div className="mb-4">
        <h2 className="text-sm font-semibold text-fg">System identity</h2>
        <p className="mt-1 text-[13px] text-fg-muted">
          Shippin system ID: <span className="font-mono">{identity.data.server_id}</span>
          {identity.data.hostname && <> · Hostname: <span className="font-medium text-fg">{identity.data.hostname}</span> ({identity.data.hostname_source}{identity.data.hostname_observed_at ? `, observed ${new Date(identity.data.hostname_observed_at).toLocaleString()}` : ""})</>}
          {identity.data.identity_updated_at && <> · Metadata updated by {identity.data.identity_updated_by} at {new Date(identity.data.identity_updated_at).toLocaleString()}</>}
        </p>
        <p className="mt-1 text-[13px] text-fg-muted">AgentHub IDs are explicit references only. This mapping does not yet validate against AgentHub or run its change-review workflow.</p>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <TextField label="Display name / nickname" value={label} onChange={setLabel} />
        <TextField label="Aliases (one per line)" multiline value={aliases} onChange={setAliases} />
        <TextField label="Tags (one per line)" multiline value={tags} onChange={setTags} />
        <TextField label="AgentHub tenant ID" value={tenantId} onChange={setTenantId} />
        <TextField label="AgentHub machine ID" value={machineId} onChange={setMachineId} />
      </div>
      <div className="mt-3 flex flex-wrap items-end gap-3">
        <TextField label="Reason for identity change" value={note} onChange={setNote} />
        <Button size="sm" variant="primary" tooltip="Save display name, aliases, tags, and an explicit AgentHub mapping" isDisabled={save.isPending || note.trim().length < 5 || !label.trim() || (!!tenantId.trim() !== !!machineId.trim())}
          onPress={() => save.mutate({ label, aliases: split(aliases), tags: split(tags), agenthub_tenant_id: tenantId.trim(), agenthub_machine_id: machineId.trim(), note })}>
          Save identity
        </Button>
        {save.isError && <p role="alert" className="text-sm text-danger">{save.error.message}</p>}
      </div>
    </Card>
  );
}

/** Everything about one provider's hosts. What it offers comes from the driver's capabilities. */
export function ProviderTab({ provider }: { provider: InfraProvider }) {
  const t = useText();
  const [picked, setPicked] = useState("");
  const node = provider.nodes.find((n) => n.id === picked) ?? firstAnswering(provider.nodes);
  if (!node) return null;
  return (
    <>
      {provider.nodes.length > 1 && (
        <div className="mb-6 max-w-xs">
          <Select label={t("infra.provider.host")} value={node.id} onChange={setPicked} options={provider.nodes.map((n) => ({ id: n.id, label: n.label }))} />
        </div>
      )}
      {node.error && <p role="alert" className="mb-6 rounded-lg border border-danger/30 bg-danger/10 px-3 py-2 text-[13px] text-danger">{node.error}</p>}
      {node.online && <NodeFacts node={node} />}
      {node.source === "api" && <SystemIdentityForm serverId={node.id} />}
      {node.source === "agent" ? <AgentGuests node={node} /> : <ApiGuests key={node.id} provider={provider} node={node} />}
    </>
  );
}
