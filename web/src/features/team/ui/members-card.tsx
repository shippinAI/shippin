import { UserMinus } from "lucide-react";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { Select } from "../../../lib/design/components/select/select";
import type { TenantTeam } from "../../../lib/api/tenant-team-types";
import { useTeamWrites } from "../../../lib/hooks/use-tenant-team";

/** Org members with their role; owners/admins change roles or remove (owner role: owners only). */
export function MembersCard({ tenantId, team, actions }: { tenantId: string; team: TenantTeam; actions: React.ReactNode }) {
  const w = useTeamWrites(tenantId);
  const opts = team.roles.map((r) => ({ id: r, label: r }));
  return (
    <Card>
      <CardHeader title={`Members (${team.members.length})`} description="People in this tenant's organization and their role." actions={actions} />
      <ul className="divide-y divide-line" aria-label="Members">
        {team.members.map((m) => {
          const ownerLocked = m.roles.includes(team.owner_role) && !team.is_owner;
          const editable = team.can_manage && !ownerLocked;
          return (
            <li key={m.user_id} className="flex items-center gap-3 px-4 py-2.5">
              <div className="min-w-0 flex-1">
                <p className="truncate font-medium text-fg">{m.name || m.email || m.user_id}</p>
                <p className="truncate text-base text-fg-muted">{m.email}</p>
              </div>
              {editable ? (
                <div className="w-48">
                  <Select label={`Role of ${m.name || m.email}`} options={opts.filter((o) => team.is_owner || o.id !== team.owner_role || m.roles.includes(o.id))}
                    value={m.roles[0] ?? ""} onChange={(r) => w.setRoles.mutate({ user: m.user_id, roles: [r] })} />
                </div>
              ) : (
                <span className="font-mono text-xs text-fg-muted">{m.roles.join(", ")}</span>
              )}
              {editable && (
                <Button size="sm" variant="ghost" aria-label={`Remove ${m.name || m.email}`} tooltip="Remove from this tenant (audited)" isDisabled={w.remove.isPending} onPress={() => w.remove.mutate(m.user_id)}>
                  <UserMinus className="size-4" />
                </Button>
              )}
            </li>
          );
        })}
      </ul>
    </Card>
  );
}
