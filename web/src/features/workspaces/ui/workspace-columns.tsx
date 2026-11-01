import { Boxes } from "lucide-react";
import { Link } from "react-aria-components";
import type { ColumnDef } from "../../../lib/design/components/data-table/data-table";
import type { Workspace } from "../../../lib/api/types";
import { timeAgo } from "../../../lib/format";
import { OpenButton } from "./open-button";
import { StateBadge } from "./state-badge";
import { WorkspaceMenu } from "./workspace-menu";

const mono = "font-mono text-[12.5px]";

export function workspaceColumns(canManage: boolean): ColumnDef<Workspace>[] {
  return [
    {
      id: "name",
      label: "Name",
      isRowHeader: true,
      render: (w) => (
        <Link href={`/t/${w.tenant_id}/workspaces/${w.id}`} className="flex items-center gap-3 outline-none">
          <span className="flex size-8 items-center justify-center rounded-lg bg-hue-workspaces/15 ring-1 ring-hue-workspaces/25">
            <Boxes className="size-4 text-hue-workspaces" strokeWidth={1.75} />
          </span>
          <span>
            <span className="block font-medium text-fg">{w.name}</span>
            <span className="block text-[11.5px] text-fg-subtle">{w.host}</span>
          </span>
        </Link>
      ),
    },
    { id: "state", label: "Status", render: (w) => <StateBadge state={w.state} /> },
    { id: "pub", label: "Public IPv4", render: (w) => <span className={mono}>{w.public_ip || "—"}</span> },
    { id: "priv", label: "Private IPv4", render: (w) => <span className={mono}>{w.ipv4 || "—"}</span> },
    { id: "region", label: "Region", render: (w) => w.region },
    { id: "created", label: "Created", render: (w) => timeAgo(w.created_at) },
    {
      id: "actions",
      label: "",
      width: "150px",
      render: (w) => (
        <div className="flex items-center justify-end gap-2">
          <OpenButton ws={w} />
          <WorkspaceMenu ws={w} canManage={canManage} />
        </div>
      ),
    },
  ];
}
