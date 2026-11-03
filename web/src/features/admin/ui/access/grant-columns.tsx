import { CircleX } from "lucide-react";
import { IconButton } from "../../../../lib/design/components/button/icon-button";
import type { ColumnDef } from "../../../../lib/design/components/data-table/data-table";
import type { Grant } from "../../../../lib/api/access-types";
import { GrantStatusBadge, grantScope, humanDuration, untilLabel } from "../../../access/ui/grant-meta";

/** Provider view of requests and grants; the requester can cancel or end their own. */
export const grantColumns = (me: string, end: (g: Grant) => void, busy: boolean): ColumnDef<Grant>[] => [
  {
    id: "tenant", label: "Tenant", isRowHeader: true, render: (g) => (
      <div className="min-w-0">
        <div className="truncate font-medium text-fg">{g.tenant_name}</div>
        <div className="truncate text-xs text-fg-subtle">{grantScope(g)}</div>
      </div>
    ),
  },
  { id: "status", label: "Status", width: "190px", render: (g) => <GrantStatusBadge grant={g} /> },
  { id: "who", label: "Admin", render: (g) => <span className="text-fg">{g.requester_name}</span> },
  { id: "reason", label: "Reason", render: (g) => <span className="line-clamp-2 text-[12.5px]" title={g.reason}>{g.category ? `[${g.category}] ` : ""}{g.reason}</span> },
  {
    id: "when", label: "Window", width: "170px", render: (g) => (
      <span className="text-xs">
        {g.status === "approved" ? untilLabel(g.expires_at) : g.status === "pending" ? `asks ${humanDuration(g.duration_s)}` : new Date(g.ended_at ?? g.decided_at ?? g.created_at).toLocaleString()}
        {g.decided_name && g.kind === "request" && g.status !== "pending" && <span className="block text-fg-subtle">by {g.decided_name}</span>}
      </span>
    ),
  },
  {
    id: "act", label: "", width: "56px", render: (g) =>
      g.requester === me && (g.status === "pending" || g.status === "approved") ? (
        <IconButton icon={CircleX} label={g.status === "pending" ? "Cancel this request" : "End this access now"} isDisabled={busy} onPress={() => end(g)} />
      ) : null,
  },
];
