import type { ColumnDef } from "../../../lib/design/components/data-table/data-table";
import type { AdminTenant } from "../../../lib/api/admin-types";
import { timeAgo } from "../../../lib/format";
import { StatusBadge } from "./status-badge";

const mono = "font-mono text-xs";

export const tenantColumns: ColumnDef<AdminTenant>[] = [
  {
    id: "name", label: "Tenant", isRowHeader: true, render: (t) => (
      <div className="min-w-0">
        <div className="truncate font-medium text-fg">{t.name}</div>
        <div className={`truncate text-fg-subtle ${mono}`}>{t.id}</div>
      </div>
    ),
  },
  {
    id: "owner", label: "Owner", render: (t) => t.owner ? (
      <div className="min-w-0">
        <div className="truncate text-fg">{t.owner}</div>
        <div className="truncate text-xs text-fg-subtle">{t.owner_email}</div>
      </div>
    ) : <span className="text-fg-subtle">—</span>,
  },
  { id: "status", label: "Status", width: "130px", render: (t) => <StatusBadge status={t.status} /> },
  { id: "subnet", label: "Subnet", width: "140px", render: (t) => <span className={mono}>{t.subnet || "—"}</span> },
  { id: "ws", label: "Workspaces", width: "110px", render: (t) => <span className="tabular-nums">{t.workspace_count}</span> },
  { id: "plan", label: "Plan", width: "90px", render: (t) => <span className="capitalize">{t.plan_id}</span> },
  { id: "created", label: "Created", width: "110px", render: (t) => <span title={new Date(t.created_at).toLocaleString()}>{timeAgo(t.created_at)}</span> },
];
