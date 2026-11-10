import { ShieldAlert, ShieldCheck } from "lucide-react";
import type { ColumnDef } from "../../../../lib/design/components/data-table/data-table";
import { Badge } from "../../../../lib/design/components/badge/badge";
import { timeAgo, initials } from "../../../../lib/format";
import type { StaffRole, TeamMember } from "../../../../lib/api/team-types";
import { RoleChips } from "./role-chips";

const stateTone = { active: "success", initial: "info", inactive: "neutral", locked: "danger" } as const;
const stateLabel: Record<string, string> = { active: "Active", initial: "Invited", inactive: "Deactivated", locked: "Locked" };

export function memberColumns(roles: StaffRole[]): ColumnDef<TeamMember>[] {
  return [
    {
      id: "name", label: "Member", isRowHeader: true, width: "30%",
      render: (m) => (
        <div className="flex items-center gap-3">
          <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-raised text-xs font-semibold text-fg">{initials(m.name)}</span>
          <div className="min-w-0">
            <div className="truncate font-medium text-fg">
              {m.name} {m.self && <span className="ml-1 text-2xs font-normal text-fg-subtle">(you)</span>}
            </div>
            <div className="truncate text-xs text-fg-subtle">{m.email || m.login}</div>
          </div>
        </div>
      ),
    },
    { id: "roles", label: "Roles", render: (m) => <RoleChips keys={m.roles} roles={roles} /> },
    {
      id: "mfa", label: "MFA", width: "110px",
      render: (m) =>
        m.kind !== "human" ? <span className="text-fg-subtle">Machine</span> : m.mfa ? (
          <span className="inline-flex items-center gap-1 text-success"><ShieldCheck className="size-3.5" /> On</span>
        ) : (
          <span className="inline-flex items-center gap-1 text-warning"><ShieldAlert className="size-3.5" /> {m.roles.length ? "Required" : "Off"}</span>
        ),
    },
    {
      id: "seen", label: "Last active", width: "140px",
      render: (m) => (
        <span className={m.idle ? "text-warning" : ""}>
          {m.last_seen ? timeAgo(m.last_seen) : m.roles.length ? "Never" : "—"}
          {m.idle && <span className="ml-1.5 text-2xs">idle</span>}
        </span>
      ),
    },
    { id: "state", label: "Status", width: "120px", render: (m) => <Badge tone={stateTone[m.state as keyof typeof stateTone] ?? "neutral"}>{stateLabel[m.state] ?? m.state}</Badge> },
  ];
}
