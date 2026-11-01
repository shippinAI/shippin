import { Trash2, X } from "lucide-react";
import { useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { TextField } from "../../../lib/design/components/field/text-field";
import { Select } from "../../../lib/design/components/select/select";
import type { TenantTeam } from "../../../lib/api/tenant-team-types";
import { useTeamWrites } from "../../../lib/hooks/use-tenant-team";

/** Teams group org members inside the tenant (stored by Shippin; Nomen has no teams). */
export function TeamsCard({ tenantId, team }: { tenantId: string; team: TenantTeam }) {
  const w = useTeamWrites(tenantId);
  const [name, setName] = useState("");
  const label = (id: string) => team.members.find((m) => m.user_id === id)?.name || id;
  const valid = /^[a-z][a-z0-9-]{0,38}[a-z0-9]$/.test(name);
  return (
    <Card>
      <CardHeader title={`Teams (${team.teams.length})`} description="Group members, for example by project or function." />
      {team.can_manage && (
        <div className="flex items-end gap-2 px-4 pb-3">
          <div className="flex-1"><TextField label="New team" value={name} onChange={setName} placeholder="backend" description="Lowercase letters, digits and dashes" /></div>
          <Button size="sm" variant="secondary" isDisabled={!valid || w.createTeam.isPending} tooltip="Create the team (audited)"
            onPress={() => w.createTeam.mutate({ name, description: "" }, { onSuccess: () => setName("") })}>Create</Button>
        </div>
      )}
      <ul className="divide-y divide-border" aria-label="Teams">
        {team.teams.map((t) => {
          const outside = team.members.filter((m) => !t.members.some((x) => x.user_id === m.user_id));
          return (
            <li key={t.id} className="px-4 py-3">
              <div className="flex items-center justify-between">
                <p className="font-medium text-fg">{t.name}</p>
                {team.can_manage && (
                  <Button size="sm" variant="ghost" aria-label={`Delete team ${t.name}`} tooltip="Delete the team (audited)" onPress={() => w.deleteTeam.mutate(t.id)}><Trash2 className="size-4" /></Button>
                )}
              </div>
              <div className="mt-1 flex flex-wrap gap-1.5">
                {t.members.map((m) => (
                  <span key={m.user_id} className="inline-flex items-center gap-1 rounded-full bg-overlay px-2 py-0.5 text-[12px]">
                    {label(m.user_id)}{m.role === "maintainer" ? " · maintainer" : ""}
                    {team.can_manage && (
                      <Button tooltip="Remove from team" size="sm" variant="ghost" aria-label={`Remove ${label(m.user_id)} from ${t.name}`} className="focus-ring rounded-full text-fg-subtle data-hovered:text-fg" onPress={() => w.removeFromTeam.mutate({ team: t.id, user: m.user_id })}><X className="size-3" /></Button>
                    )}
                  </span>
                ))}
              </div>
              {team.can_manage && outside.length > 0 && (
                <div className="mt-2 w-64">
                  <Select label={`Add to ${t.name}`} options={outside.map((m) => ({ id: m.user_id, label: m.name || m.email }))} value=""
                    onChange={(u) => w.addToTeam.mutate({ team: t.id, user: u, role: "member" })} />
                </div>
              )}
            </li>
          );
        })}
      </ul>
    </Card>
  );
}
