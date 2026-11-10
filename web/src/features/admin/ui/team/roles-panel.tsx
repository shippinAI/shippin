import { Plus, ShieldAlert, Users } from "lucide-react";
import { useState } from "react";
import { Button as RacButton } from "react-aria-components";
import { Badge } from "../../../../lib/design/components/badge/badge";
import { Button } from "../../../../lib/design/components/button/button";
import type { Team } from "../../../../lib/api/team-types";
import { RoleEditor } from "./role-editor";

/** Every role as a card; click to edit. Presets are ordinary, editable rows. */
export function RolesPanel({ team, canEdit }: { team: Team; canEdit: boolean }) {
  const [open, setOpen] = useState<string | "new">();
  const risky = new Set(team.catalog.filter((p) => p.high_risk).map((p) => p.key));
  return (
    <>
      <div className="mb-4 flex items-center justify-between">
        <p className="text-sm text-fg-muted">{team.roles.length} roles · {team.catalog.length} permissions in the catalog. Deny by default: a role grants only what is ticked.</p>
        {canEdit && <Button variant="primary" size="sm" tooltip="Create a role and pick its permissions" onPress={() => setOpen("new")}><Plus className="size-4" /> New role</Button>}
      </div>
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
        {team.roles.map((r) => {
          const holders = team.members.filter((m) => m.roles.includes(r.key)).length;
          const high = r.permissions.filter((p) => risky.has(p)).length;
          return (
            <RacButton key={r.key} onPress={() => setOpen(r.key)}
              className="focus-ring group rounded-xl border border-line bg-surface p-4 text-left shadow-e1 outline-none transition-colors data-hovered:border-line-strong data-hovered:bg-raised/40">
              <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                  <div className="truncate text-lg font-semibold tracking-tight text-fg">{r.name}</div>
                  <div className="truncate font-mono text-xs text-fg-subtle">{r.key}</div>
                </div>
                {r.permissions.includes("team.write") ? <Badge tone="warning">Owner</Badge> : r.preset ? <Badge>Preset</Badge> : <Badge tone="info">Custom</Badge>}
              </div>
              <p className="mt-2 line-clamp-2 min-h-9 text-sm text-fg-muted">{r.description || "No description."}</p>
              <div className="mt-3 flex items-center gap-4 text-xs text-fg-subtle">
                <span className="tabular-nums">{r.permissions.length} permissions</span>
                {high > 0 && <span className="inline-flex items-center gap-1 text-warning"><ShieldAlert className="size-3.5" /> {high} high-risk</span>}
                <span className="ml-auto inline-flex items-center gap-1 tabular-nums"><Users className="size-3.5" /> {holders}</span>
              </div>
            </RacButton>
          );
        })}
      </div>
      {open && <RoleEditor key={open} team={team} role={open === "new" ? undefined : team.roles.find((r) => r.key === open)} canEdit={canEdit} onClose={() => setOpen(undefined)} />}
    </>
  );
}
