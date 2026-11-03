import { useState } from "react";
import { Cpu, Plus } from "lucide-react";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { DataTable, type ColumnDef } from "../../../lib/design/components/data-table/data-table";
import { ModalDialog } from "../../../lib/design/components/dialog/modal-dialog";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { TextField } from "../../../lib/design/components/field/text-field";
import { Select } from "../../../lib/design/components/select/select";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import { StatusDot } from "../../../lib/design/components/status-dot/status-dot";
import { ApiError } from "../../../lib/api/client";
import { useServices } from "../../../lib/hooks/use-api";
import { useCreateGuest, useGuestAction, useGuests, useRollback, useSnapshot, useSnapshots, useTask, type Guest } from "../../../lib/hooks/use-hosts";
import { useDetachHost, type InfraNode, type InfraProvider } from "../../../lib/hooks/use-infra";
import { useNavigate } from "../../../lib/router/router";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { firstAnswering, isRunning, isStopped, stoppableGuests } from "../model";
import { ActivityCard } from "./activity-card";
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

function BoardNotice({ error }: { error: Error | null }) {
  const nav = useNavigate();
  const t = useText();
  if (!(error instanceof ApiError) || error.code !== "waiting_on_board") return null;
  return (
    <Card className="mb-6 p-5">
      <h2 className="mb-1 text-sm font-semibold text-fg">{t("infra.board.title")}</h2>
      <p className="text-sm text-fg-muted">{error.message}</p>
      {error.cardKey && <p className="mt-2 font-mono text-[12px] text-fg">{error.cardKey}</p>}
      <div className="mt-3">
        <Button size="sm" variant="primary" tooltip={t("infra.board.openTooltip")} onPress={() => nav("/hub")}>{t("infra.board.open")}</Button>
      </div>
    </Card>
  );
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
  const snap = useSnapshot(base);
  const back = useRollback(base);
  const create = useCreateGuest(node.id);
  const detach = useDetachHost();
  const [confirming, setConfirming] = useState(false);
  const [stopping, setStopping] = useState(false);
  const [selfVerb, setSelfVerb] = useState<"stop" | "shutdown" | "reboot" | null>(null);
  const [snapName, setSnapName] = useState("");
  const [kind, setKind] = useState("ct");
  const [newId, setNewId] = useState("");
  const [newName, setNewName] = useState("");
  const [upid, setUpid] = useState("");
  const [lookup, setLookup] = useState("");
  const task = useTask(can("tasks") ? base : "", lookup);
  const notice = [act.error, snap.error, back.error, create.error].find((e) => e instanceof ApiError && e.code === "waiting_on_board") ?? null;
  const guestKinds = [{ id: "ct", label: t("infra.guest.ct") }, { id: "vm", label: t("infra.guest.vm") }];
  return (
    <>
      <BoardNotice error={notice} />
      <Card elevation={2} className="mb-6 overflow-hidden">
        <CardHeader
          title={node.label}
          description={[node.base_url, node.scope, node.version].filter(Boolean).join(" · ")}
          actions={
            <div className="flex gap-2">
              {can("lifecycle") && (
                <Button size="sm" variant="secondary" tooltip={t("infra.stop.tooltip")} isDisabled={stoppableGuests(guests.data?.items ?? []).length === 0} onPress={() => setStopping(true)}>{t("infra.stop.open")}</Button>
              )}
              <Button size="sm" variant="ghost" tooltip={t("infra.detach.tooltip")} onPress={() => setConfirming(true)}>{t("infra.detach")}</Button>
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
      {guest && (can("lifecycle") || can("snapshot")) && (
        <Card className="mb-6 p-5">
          <h2 className="mb-3 text-sm font-semibold text-fg">{guest.name || guest.id}</h2>
          {can("lifecycle") && (
            <div className="mb-4 flex flex-wrap gap-2">
              {(provider.verbs as ("start" | "stop" | "shutdown" | "reboot")[]).map((verb) => (
                <Button key={verb} size="sm" variant="secondary" tooltip={t("infra.guest.verbTooltip", { verb, guest: guest.id })} isDisabled={act.isPending}
                  onPress={() => (guest.self && verb !== "start" ? setSelfVerb(verb) : act.mutate({ guest: guest.id, verb }))}>
                  {verb}
                </Button>
              ))}
            </div>
          )}
          {can("snapshot") && (
            <div className="mb-4 flex flex-wrap items-end gap-2">
              <TextField label={t("infra.snapshot.name")} value={snapName} onChange={setSnapName} description={t("infra.snapshot.help")} />
              <Button size="sm" variant="primary" tooltip={t("infra.snapshot.takeTooltip")} isDisabled={!snapName || snap.isPending}
                onPress={() => snap.mutate({ guest: guest.id, snapname: snapName })}>
                {t("infra.snapshot.take")}
              </Button>
            </div>
          )}
          {can("snapshot_list") && (
            <ul className="space-y-2 text-sm">
              {(snaps.data?.items ?? []).map((name) => (
                <li key={name} className="flex items-center justify-between gap-3">
                  <span className="font-mono">{name}</span>
                  {can("rollback") && (
                    <Button size="sm" variant="ghost" tooltip={t("infra.snapshot.rollbackTooltip", { guest: guest.id, name })} isDisabled={back.isPending}
                      onPress={() => back.mutate({ guest: guest.id, name })}>
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
      {(can("create") || can("tasks")) && (
        <div className="grid gap-6 lg:grid-cols-2">
          {can("create") && (
            <Card className="p-5">
              <h2 className="mb-3 text-sm font-semibold text-fg">{t("infra.create.title")}</h2>
              <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); create.mutate({ kind, id: newId, name: newName }); }}>
                <Select label={t("infra.create.kind")} options={guestKinds} value={kind} onChange={setKind} />
                <TextField label={t("infra.create.id")} value={newId} onChange={setNewId} />
                <TextField label={t("infra.create.name")} value={newName} onChange={setNewName} />
                <Button type="submit" variant="primary" size="sm" tooltip={t("infra.create.tooltip")} isDisabled={!newId || create.isPending}>
                  <Plus className="size-4" /> {t("infra.create.submit")}
                </Button>
              </form>
            </Card>
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
      <ModalDialog isOpen={selfVerb !== null} onOpenChange={(o) => { if (!o) setSelfVerb(null); }} title={t("infra.self.title")} description={t("infra.self.body")}>
        <div className="flex justify-end gap-2 px-6 py-5">
          <Button variant="ghost" tooltip={t("infra.dialog.cancelTooltip")} onPress={() => setSelfVerb(null)}>{t("infra.dialog.cancel")}</Button>
          <Button variant="primary" tooltip={t("infra.self.confirmTooltip")} isDisabled={act.isPending}
            onPress={() => { if (guest && selfVerb) act.mutate({ guest: guest.id, verb: selfVerb, confirmSelf: true }); setSelfVerb(null); }}>
            {t("infra.self.confirm", { verb: selfVerb ?? "" })}
          </Button>
        </div>
      </ModalDialog>
      <ModalDialog isOpen={confirming} onOpenChange={setConfirming} title={t("infra.detach.title", { label: node.label })} description={t("infra.detach.body")}>
        <div className="flex justify-end gap-2 px-6 py-5">
          <Button variant="ghost" tooltip={t("infra.dialog.cancelTooltip")} onPress={() => setConfirming(false)}>{t("infra.dialog.cancel")}</Button>
          <Button variant="primary" tooltip={t("infra.detach.tooltip")} isDisabled={detach.isPending}
            onPress={() => detach.mutate({ id: node.id, label: node.label }, { onSuccess: () => setConfirming(false) })}>
            {t("infra.detach.confirm")}
          </Button>
        </div>
      </ModalDialog>
    </>
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
      {node.source === "agent" ? <AgentGuests node={node} /> : <ApiGuests key={node.id} provider={provider} node={node} />}
    </>
  );
}
