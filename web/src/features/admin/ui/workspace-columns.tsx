import { KeyRound, Play, Siren, Square, Trash2 } from "lucide-react";
import { IconButton } from "../../../lib/design/components/button/icon-button";
import type { ColumnDef } from "../../../lib/design/components/data-table/data-table";
import type { AdminWorkspace } from "../../../lib/api/admin-types";
import { StatusBadge } from "./status-badge";

const mono = "font-mono text-xs";
export type AdminWsAct = "start" | "stop" | "delete" | "request" | "breakglass";
type Act = (w: AdminWorkspace, a: AdminWsAct) => void;

export const workspaceColumns = (act: Act, busy: boolean): ColumnDef<AdminWorkspace>[] => [
  {
    id: "name", label: "Workspace", isRowHeader: true, render: (w) => (
      <div className="min-w-0">
        <div className="truncate font-medium text-fg">{w.name}</div>
        <div className={`truncate text-fg-subtle ${mono}`}>{w.id.slice(0, 8)}…</div>
      </div>
    ),
  },
  { id: "tenant", label: "Tenant", render: (w) => <span className="text-fg">{w.tenant_name}</span> },
  { id: "owner", label: "Owner", render: (w) => <span className="truncate text-xs">{w.created_by || "—"}</span> },
  { id: "state", label: "State", width: "130px", render: (w) => <StatusBadge status={w.state} /> },
  { id: "ip", label: "Private IP", width: "120px", render: (w) => <span className={mono}>{w.ipv4 || "—"}</span> },
  { id: "ssh", label: "SSH", width: "160px", render: (w) => <span className={mono}>{w.public_ip ? `${w.public_ip}:${w.ssh_port}` : w.ssh_port}</span> },
  {
    id: "url", label: "Public URL", render: (w) => (
      <a href={w.url} target="_blank" rel="noreferrer" className="block max-w-56 truncate text-primary hover:underline" title={w.url}>
        {w.host}
      </a>
    ),
  },
  {
    id: "act", label: "", width: "184px", render: (w) => (
      <div className="flex justify-end gap-0.5">
        <IconButton icon={KeyRound} label={`Request access to ${w.name}`} isDisabled={w.tenant_status === "deleted"} onPress={() => act(w, "request")} />
        <IconButton icon={Siren} label={`Break glass on ${w.name}`} className="data-hovered:text-danger" isDisabled={w.tenant_status === "deleted"} onPress={() => act(w, "breakglass")} />
        <IconButton icon={Play} label={`Start ${w.name}`} isDisabled={busy || w.state === "running" || w.tenant_status === "suspended"} onPress={() => act(w, "start")} />
        <IconButton icon={Square} label={`Stop ${w.name}`} isDisabled={busy || w.state !== "running"} onPress={() => act(w, "stop")} />
        <IconButton icon={Trash2} label={`Delete ${w.name}`} isDisabled={busy || w.state === "deleting" || w.state === "provisioning"} onPress={() => act(w, "delete")} />
      </div>
    ),
  },
];
