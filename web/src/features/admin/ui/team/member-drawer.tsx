import { UserX, UserCheck } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import { CheckboxGroup, Label } from "react-aria-components";
import { Button } from "../../../../lib/design/components/button/button";
import { Checkbox } from "../../../../lib/design/components/checkbox/checkbox";
import { Drawer } from "../../../../lib/design/components/drawer/drawer";
import { TextField } from "../../../../lib/design/components/field/text-field";
import type { Team, TeamMember } from "../../../../lib/api/team-types";
import { useSetActive, useSetRoles } from "../../../../lib/hooks/use-team";
import { RoleChips } from "./role-chips";
import { useIdpName } from "../../../../lib/hooks/use-idp-name";

type Props = { team: Team; member?: TeamMember; canWrite: boolean; onClose: () => void };

/** One staff member: roles (checkboxes), account state. Every save needs a reason (audited). */
export function MemberDrawer({ team, member, canWrite, onClose }: Props) {
  const idp = useIdpName();
  const [roles, setRoles] = useState<string[]>([]);
  const [reason, setReason] = useState("");
  const save = useSetRoles();
  const active = useSetActive();
  useEffect(() => {
    setRoles(member?.roles ?? []);
    setReason("");
  }, [member]);
  if (!member) return null;
  const changed = roles.length !== member.roles.length || roles.some((r) => !member.roles.includes(r));
  const okReason = reason.trim().length >= 5;
  const lastOwner = member.owner && team.owners <= 1;
  const deactivated = member.state === "inactive";
  return (
    <Drawer
      isOpen
      onOpenChange={(o) => !o && onClose()}
      title={member.name}
      subtitle={member.email || member.login}
      footer={
        canWrite && (
          <>
            <Button variant="primary" isDisabled={!changed || !okReason || save.isPending} tooltip={!changed ? "Change a role first" : okReason ? `Save the roles (audited first, then sent to ${idp})` : "Add a reason"}
              onPress={() => save.mutate({ id: member.id, roles, reason: reason.trim() }, { onSuccess: onClose })}>Save roles</Button>
            {!member.self && (
              <Button variant={deactivated ? "secondary" : "danger"} className="ml-auto" isDisabled={!okReason || active.isPending || (!deactivated && lastOwner)}
                tooltip={lastOwner && !deactivated ? "The last Owner can't be deactivated" : okReason ? (deactivated ? "Allow sign-in again" : "Block sign-in; history is kept") : "Add a reason"}
                onPress={() => active.mutate({ id: member.id, state: deactivated ? "reactivate" : "deactivate", reason: reason.trim() }, { onSuccess: onClose })}>
                {deactivated ? <UserCheck className="size-4" /> : <UserX className="size-4" />} {deactivated ? "Reactivate" : "Deactivate"}
              </Button>
            )}
          </>
        )
      }
    >
      <div className="space-y-6 px-6 py-5">
        <div className="grid grid-cols-2 gap-4">
          <Fact label="Status" value={member.state} />
          <Fact label="MFA" value={member.mfa ? "Enrolled" : member.kind === "human" ? "Not enrolled — staff permissions stay off" : "Machine user (no MFA)"} />
          <Fact label="Last active" value={member.last_seen ? new Date(member.last_seen).toLocaleString() : "Never in the console"} />
          <Fact label="Current roles" value={<RoleChips keys={member.roles} roles={team.roles} />} />
        </div>
        <CheckboxGroup value={roles} onChange={setRoles} isDisabled={!canWrite} className="space-y-1">
          <Label className="text-sm font-medium text-fg">Roles</Label>
          {member.self && <p className="pb-1 text-xs text-warning">You can remove your own roles, but not add any; ask another Owner.</p>}
          {team.roles.map((r) => (
            <Checkbox key={r.key} value={r.key} isDisabled={!canWrite || (member.self && !member.roles.includes(r.key))}
              description={`${r.description || r.key} · ${r.permissions.length} permissions`}>
              {r.name}
            </Checkbox>
          ))}
        </CheckboxGroup>
        {canWrite && (
          <TextField label="Reason" multiline value={reason} onChange={setReason} placeholder="e.g. Joining the on-call rotation" description="Required; kept in the audit log with the change." />
        )}
        {lastOwner && <p className="text-xs text-fg-subtle">This is the only active Owner. Make someone else an Owner before removing this role.</p>}
      </div>
    </Drawer>
  );
}

function Fact({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div>
      <div className="text-xs font-medium tracking-wide text-fg-subtle uppercase">{label}</div>
      <div className="mt-1 text-sm text-fg">{value}</div>
    </div>
  );
}
