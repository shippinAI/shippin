import { Siren } from "lucide-react";
import { DataTable, type ColumnDef } from "../../../lib/design/components/data-table/data-table";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import type { AuditEntry } from "../../../lib/api/access-types";
import { cn } from "../../../lib/design/components/cn/cn";

const labels: Record<string, string> = {
  request: "Requested access", approve: "Approved", deny: "Denied", revoke: "Revoked", cancel: "Cancelled request",
  end: "Ended access", expire: "Expired", ide: "Opened IDE", preview: "Opened preview", action: "Workspace action",
  change: "Changed settings", activate: "Activated break-glass", notice_withheld: "Customer notice delayed",
};

export const actionLabel = (a: string) => labels[a.replace(/^(access|breakglass)\./, "")] ?? a;

type Row = Omit<AuditEntry, "id"> & { id: string };

const cols = (showTenant: boolean): ColumnDef<Row>[] => [
  {
    id: "at", label: "Time", width: "170px", isRowHeader: true,
    render: (e) => <span className="font-mono text-[12px] text-fg-muted">{new Date(e.at).toLocaleString()}</span>,
  },
  {
    id: "action", label: "Event", width: "220px", render: (e) => (
      <span className={cn("inline-flex items-center gap-1.5 font-medium", e.breakglass ? "text-danger" : "text-fg")}>
        {e.breakglass && <Siren className="size-3.5" aria-label="Break-glass" />}
        {actionLabel(e.action)}
      </span>
    ),
  },
  { id: "actor", label: "Who", render: (e) => <span className="text-fg">{e.actor_name || e.actor}</span> },
  ...(showTenant ? [{ id: "tenant", label: "Tenant", render: (e: Row) => <span>{e.tenant_name || "—"}</span> }] : []),
  { id: "ws", label: "Workspace", render: (e) => <span>{e.workspace_name || "All"}</span> },
  { id: "detail", label: "Detail", render: (e) => <span className="line-clamp-2 text-[12.5px] break-words" title={e.detail}>{e.detail || "—"}</span> },
  { id: "ip", label: "IP", width: "130px", render: (e) => <span className="font-mono text-[12px]">{e.ip || "—"}</span> },
];

/** Append-only audit log; break-glass entries are red with a siren. */
export function AuditTable({ items, showTenant = false }: { items: AuditEntry[]; showTenant?: boolean }) {
  const rows = items.map((e) => ({ ...e, id: String(e.id) }));
  return (
    <div className="[&_tr:has([aria-label=Break-glass])]:bg-danger/[0.07] [&_tr:has([aria-label=Break-glass])_td:first-child]:shadow-[inset_3px_0_0_var(--color-danger)]">
      <DataTable
        label="Audit log"
        dense
        columns={cols(showTenant)}
        rows={rows}
        empty={<EmptyState title="No audit entries yet" description="Access requests, decisions and every admin access appear here. Entries can't be edited or deleted." />}
      />
    </div>
  );
}
