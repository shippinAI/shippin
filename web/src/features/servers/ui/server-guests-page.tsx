import { useState } from "react";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { DataTable, type ColumnDef } from "../../../lib/design/components/data-table/data-table";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { TextField } from "../../../lib/design/components/field/text-field";
import { StatusDot } from "../../../lib/design/components/status-dot/status-dot";
import { useGuests, useRollback, useSnapshot, useSnapshots, useTask, type Guest } from "../../../lib/hooks/use-hosts";
import { useNavigate, useParams } from "../../../lib/router/router";
import { PageHeader } from "../../../template/page-header";
import { PowerReviewDialog } from "../../infrastructure/ui/power-review-dialog";

const columns: ColumnDef<Guest>[] = [
  { id: "name", label: "Name", isRowHeader: true, render: (g) => <span className="font-medium text-fg">{g.name || g.id}</span> },
  { id: "id", label: "ID", render: (g) => <span className="font-mono text-sm">{g.id}</span> },
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

/** Guests on one server this organization owns or is assigned. */
export function ServerGuestsPage() {
  const nav = useNavigate();
  const { tenantId = "", serverId = "" } = useParams();
  const base = tenantId && serverId ? `/v1/tenants/${tenantId}/servers/${serverId}` : "";
  const guests = useGuests(base);
  const [guestId, setGuestId] = useState("");
  const guest = (guests.data?.items ?? []).find((g) => g.id === guestId);
  const snaps = useSnapshots(base, guest?.id ?? "");
  const snap = useSnapshot(base);
  const back = useRollback(base);
  const [snapName, setSnapName] = useState("");
  const [upid, setUpid] = useState("");
  const [lookup, setLookup] = useState("");
  const [powerChange, setPowerChange] = useState<"start" | "stop" | "shutdown" | "reboot" | null>(null);
  const task = useTask(base, lookup);
  return (
    <>
      <PageHeader
        eyebrow="Servers"
        title="Guests"
        description="Machines on this server. Guest power changes require a current AgentHub review."
        actions={<Button variant="ghost" size="sm" tooltip="Back to the server list" onPress={() => nav(`/t/${tenantId}/servers`)}>All servers</Button>}
      />
      <Card elevation={2} className="mb-6 overflow-hidden">
        <CardHeader title="Guests" description={guests.isError ? guests.error.message : "Select a row to change it."} />
        <DataTable label="Guests" columns={columns} rows={guests.data?.items ?? []} onRowAction={setGuestId}
          empty={<EmptyState title={guests.isError ? "Can't list guests" : "No guests"} description={guests.error?.message ?? "This server has no guests yet."} />} />
      </Card>
      {guest && (
        <Card className="mb-6 p-5">
          <h2 className="mb-3 text-base font-semibold text-fg">{guest.name || guest.id}</h2>
          <div className="mb-4 flex flex-wrap gap-2">
            {(["start", "stop", "shutdown", "reboot"] as const).map((verb) => (
              <Button key={verb} size="sm" variant="secondary" tooltip={`${verb} ${guest.id} after AgentHub review.`} isDisabled={!!powerChange}
                onPress={() => setPowerChange(verb)}>
                {verb}
              </Button>
            ))}
          </div>
          <div className="mb-4 flex flex-wrap items-end gap-2">
            <TextField label="Snapshot name" value={snapName} onChange={setSnapName} />
            <Button size="sm" variant="primary" tooltip="Snapshot this guest" isDisabled={!snapName || snap.isPending}
              onPress={() => snap.mutate({ guest: guest.id, snapname: snapName })}>
              Snapshot
            </Button>
          </div>
          <ul className="space-y-2 text-base">
            {(snaps.data?.items ?? []).map((name) => (
              <li key={name} className="flex items-center justify-between gap-3">
                <span className="font-mono">{name}</span>
                <Button size="sm" variant="ghost" tooltip={`Roll back to ${name}`} isDisabled={back.isPending} onPress={() => back.mutate({ guest: guest.id, name })}>Roll back</Button>
              </li>
            ))}
          </ul>
        </Card>
      )}
      {guest && powerChange && <PowerReviewDialog base={base} guest={guest.id} verb={powerChange} confirmSelf={!!guest.self && powerChange !== "start"} onClose={() => setPowerChange(null)} onExecuted={() => { void guests.refetch(); }} />}
      <Card className="p-5">
        <h2 className="mb-3 text-base font-semibold text-fg">Task</h2>
        <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); setLookup(upid); }}>
          <TextField label="Task id" value={upid} onChange={setUpid} description="Paste a task id from an error or from the operator. The page refreshes until it stops." />
          <Button type="submit" size="sm" variant="secondary" tooltip="Read this task" isDisabled={!upid}>Look up</Button>
        </form>
        {task.data && <pre className="mt-4 max-h-48 overflow-auto whitespace-pre-wrap font-mono text-xs text-fg-muted">{task.data.status}{task.data.done ? " · done" : ""}{"\n"}{(task.data.log ?? []).join("\n")}</pre>}
        {task.isError && <p className="mt-3 text-base text-danger">{task.error.message}</p>}
      </Card>
    </>
  );
}
