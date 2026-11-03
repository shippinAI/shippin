import { useState } from "react";
import { Cpu, Plus } from "lucide-react";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { DataTable, type ColumnDef } from "../../../lib/design/components/data-table/data-table";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { TextField } from "../../../lib/design/components/field/text-field";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import { StatusDot } from "../../../lib/design/components/status-dot/status-dot";
import { ApiError } from "../../../lib/api/client";
import { useAttach, useCreateGuest, useGuestAction, useGuests, useHosts, useRegistry, useRollback, useSnapshot, useSnapshots, useTask, type Guest } from "../../../lib/hooks/use-hosts";
import { useNavigate } from "../../../lib/router/router";
import { PageHeader } from "../../../template/page-header";

const columns: ColumnDef<Guest>[] = [
  { id: "name", label: "Name", isRowHeader: true, render: (g) => <span className="font-medium text-fg">{g.name || g.id}</span> },
  { id: "id", label: "ID", render: (g) => <span className="font-mono text-[12.5px]">{g.id}</span> },
  { id: "kind", label: "Kind", render: (g) => <span className="capitalize">{g.kind === "ct" ? "Container" : "Virtual machine"}</span> },
  {
    id: "status",
    label: "Status",
    render: (g) => (
      <Badge tone={g.status === "running" ? "success" : g.status === "stopped" ? "neutral" : "warning"}>
        <StatusDot health={g.status === "running" ? "up" : "unknown"} /> {g.status}
      </Badge>
    ),
  },
  { id: "node", label: "Node", render: (g) => g.node },
];

function BoardNotice({ error }: { error: Error | null }) {
  const nav = useNavigate();
  if (!(error instanceof ApiError) || error.code !== "waiting_on_board") return null;
  return (
    <Card className="mb-6 p-5">
      <h2 className="mb-1 text-sm font-semibold text-fg">Waiting on the factory board</h2>
      <p className="text-sm text-fg-muted">{error.message}</p>
      {error.cardKey && <p className="mt-2 font-mono text-[12px] text-fg">{error.cardKey}</p>}
      <div className="mt-3">
        <Button size="sm" variant="primary" tooltip="Open the factory board to approve this card, then retry the same action" onPress={() => nav("/hub")}>
          Open the board
        </Button>
      </div>
    </Card>
  );
}

export function HostsPage() {
  const hosts = useHosts();
  const registry = useRegistry();
  const attach = useAttach();
  const items = hosts.data?.items ?? [];
  const [picked, setPicked] = useState("");
  const host = items.find((h) => h.id === picked) ?? items[0];
  const id = host?.id ?? "";
  const base = id ? `/v1/infra/servers/${id}` : "";
  const guests = useGuests(base);
  const [guestId, setGuestId] = useState("");
  const guest = (guests.data?.items ?? []).find((g) => g.id === guestId);
  const snaps = useSnapshots(base, guest?.id ?? "");
  const act = useGuestAction(base);
  const snap = useSnapshot(base);
  const back = useRollback(base);
  const create = useCreateGuest(id);
  const [snapName, setSnapName] = useState("");
  const [kind, setKind] = useState("ct");
  const [newId, setNewId] = useState("");
  const [newName, setNewName] = useState("");
  const [upid, setUpid] = useState("");
  const [lookup, setLookup] = useState("");
  const [attachId, setAttachId] = useState("");
  const [baseURL, setBaseURL] = useState("");
  const [node, setNode] = useState("");
  const [tokenID, setTokenID] = useState("");
  const [tokenEnv, setTokenEnv] = useState("SHIPPIN_HV_TOKEN");
  const task = useTask(base, lookup);
  const notice = [act.error, snap.error, back.error, create.error, attach.error].find((e) => e instanceof ApiError && e.code === "waiting_on_board") ?? null;

  return (
    <>
      <PageHeader
        eyebrow="Operator"
        title="Hosts"
        description="Guests on a server's hypervisor. The same start, stop, snapshot, and task model for every driver. Phase 1 speaks Proxmox. Incus guests stay on Instances until that driver shares this page."
      />
      <BoardNotice error={notice} />
      <Card elevation={2} className="mb-6 overflow-hidden">
        <CardHeader
          title={host ? host.label || host.slug : "Servers"}
          description={host ? `${host.kind} · node ${host.node}` : "A hypervisor API is columns on a server row. Nothing is attached yet."}
          actions={
            items.length > 1 ? (
              <select aria-label="Server" className="h-9 rounded-md border border-line bg-canvas px-2 text-sm" value={id} onChange={(e) => setPicked(e.target.value)}>
                {items.map((h) => <option key={h.id} value={h.id}>{h.label || h.slug}</option>)}
              </select>
            ) : undefined
          }
        />
        {hosts.isPending || guests.isPending ? (
          <div className="space-y-3 p-5">{Array.from({ length: 4 }, (_, i) => <Skeleton key={i} className="h-9" />)}</div>
        ) : (
          <DataTable
            label="Guests"
            columns={columns}
            rows={guests.data?.items ?? []}
            onRowAction={setGuestId}
            empty={
              <EmptyState
                illustration={<Cpu className="size-10 text-fg-subtle" />}
                title={hosts.isError || guests.isError ? "Can't list guests" : "No hypervisor attached"}
                description={hosts.error?.message ?? guests.error?.message ?? "Attach a Proxmox API to a server from the registry. The token secret stays in the environment variable named on that row."}
              />
            }
          />
        )}
      </Card>
      {guest && (
        <Card className="mb-6 p-5">
          <h2 className="mb-3 text-sm font-semibold text-fg">{guest.name || guest.id}</h2>
          <div className="mb-4 flex flex-wrap gap-2">
            {(["start", "stop", "shutdown", "reboot"] as const).map((verb) => (
              <Button key={verb} size="sm" variant="secondary" tooltip={`${verb} ${guest.id}. The button waits until inventory matches.`} isDisabled={act.isPending}
                onPress={() => act.mutate({ guest: guest.id, verb })}>
                {verb}
              </Button>
            ))}
          </div>
          <div className="mb-4 flex flex-wrap items-end gap-2">
            <TextField label="Snapshot name" value={snapName} onChange={setSnapName} description="Lowercase, 2–40 characters, starting with a letter." />
            <Button size="sm" variant="primary" tooltip="Snapshot this guest, then read it back" isDisabled={!snapName || snap.isPending}
              onPress={() => snap.mutate({ guest: guest.id, snapname: snapName })}>
              Snapshot
            </Button>
          </div>
          <ul className="space-y-2 text-sm">
            {(snaps.data?.items ?? []).map((name) => (
              <li key={name} className="flex items-center justify-between gap-3">
                <span className="font-mono">{name}</span>
                <Button size="sm" variant="ghost" tooltip={`Roll ${guest.id} back to ${name}`} isDisabled={back.isPending}
                  onPress={() => back.mutate({ guest: guest.id, name })}>
                  Roll back
                </Button>
              </li>
            ))}
            {snaps.data && snaps.data.items.length === 0 && <li className="text-fg-muted">No snapshots yet.</li>}
          </ul>
        </Card>
      )}
      <Card className="mb-6 p-5">
        <h2 className="mb-1 text-sm font-semibold text-fg">Attach a hypervisor</h2>
        <p className="mb-3 text-sm text-fg-muted">Pick a server. The token secret stays in the environment variable you name. It is not saved here.</p>
        <form className="grid gap-3 md:grid-cols-2" onSubmit={(e) => { e.preventDefault(); attach.mutate({ id: attachId, base_url: baseURL, node, token_id: tokenID, token_env: tokenEnv }); }}>
          <label className="flex flex-col gap-1.5 text-[13px] font-medium text-fg md:col-span-2">
            Server
            <select aria-label="Server to attach" className="h-9 rounded-md border border-line bg-canvas px-2 text-sm font-normal" value={attachId} onChange={(e) => setAttachId(e.target.value)}>
              <option value="">Choose a server</option>
              {(registry.data?.items ?? []).map((s) => (
                <option key={s.id} value={s.id}>{s.label || s.slug}{s.kind ? ` · ${s.kind}` : ""}</option>
              ))}
            </select>
          </label>
          <TextField label="API URL" value={baseURL} onChange={setBaseURL} description="https://host:8006" />
          <TextField label="Node" value={node} onChange={setNode} />
          <TextField label="Token id" value={tokenID} onChange={setTokenID} description="user@realm!name" />
          <TextField label="Token env" value={tokenEnv} onChange={setTokenEnv} description="Name of the variable that holds the secret" />
          <div className="md:col-span-2">
            <Button type="submit" variant="primary" size="sm" tooltip="Save the API address on this server. Approve the card if the board asks, then retry." isDisabled={!attachId || !baseURL || !node || !tokenID || !tokenEnv || attach.isPending}>
              Attach
            </Button>
          </div>
        </form>
      </Card>
      <div className="grid gap-6 lg:grid-cols-2">
        <Card className="p-5">
          <h2 className="mb-3 text-sm font-semibold text-fg">New guest</h2>
          <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); create.mutate({ kind, id: newId, name: newName }); }}>
            <label className="flex flex-col gap-1.5 text-[13px] font-medium text-fg">
              Kind
              <select aria-label="Kind" className="h-9 rounded-md border border-line bg-canvas px-2 text-sm font-normal" value={kind} onChange={(e) => setKind(e.target.value)}>
                <option value="ct">Container</option>
                <option value="vm">Virtual machine</option>
              </select>
            </label>
            <TextField label="ID" value={newId} onChange={setNewId} />
            <TextField label="Name" value={newName} onChange={setNewName} />
            <Button type="submit" variant="primary" size="sm" tooltip="Create the guest and wait for the task" isDisabled={!id || !newId || create.isPending}>
              <Plus className="size-4" /> Create
            </Button>
          </form>
        </Card>
        <Card className="p-5">
          <h2 className="mb-3 text-sm font-semibold text-fg">Task</h2>
          <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); setLookup(upid); }}>
            <TextField label="Task id" value={upid} onChange={setUpid} description="The hypervisor's task id. This reads status and the log once, then refreshes until it stops." />
            <Button type="submit" size="sm" variant="secondary" tooltip="Read this task" isDisabled={!id || !upid}>Look up</Button>
          </form>
          {task.data && (
            <div className="mt-4 text-sm">
              <p className="text-fg">{task.data.status}{task.data.exitstatus ? ` · ${task.data.exitstatus}` : ""}{task.data.done ? " · done" : ""}</p>
              <pre className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap font-mono text-[12px] text-fg-muted">{(task.data.log ?? []).join("\n")}</pre>
            </div>
          )}
          {task.isError && <p className="mt-3 text-sm text-danger">{task.error.message}</p>}
        </Card>
      </div>
    </>
  );
}
