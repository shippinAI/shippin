import { Trash2 } from "lucide-react";
import { useState } from "react";
import { Button } from "../../../../lib/design/components/button/button";
import { Drawer } from "../../../../lib/design/components/drawer/drawer";
import { TextField } from "../../../../lib/design/components/field/text-field";
import type { StaffRole, Team } from "../../../../lib/api/team-types";
import { useDeleteRole, useSaveRole } from "../../../../lib/hooks/use-team";
import { PermissionPicker } from "./permission-picker";
import { useIdpName } from "../../../../lib/hooks/use-idp-name";

type Props = { team: Team; role?: StaffRole; canEdit: boolean; onClose: () => void };

/** Create or edit a role: key (new only), name, description and permissions from the catalog. */
export function RoleEditor({ team, role, canEdit, onClose }: Props) {
  const idp = useIdpName();
  const create = !role;
  const [f, setF] = useState({ key: role?.key ?? "staff.", name: role?.name ?? "", description: role?.description ?? "", reason: "" });
  const [perms, setPerms] = useState<string[]>(role?.permissions ?? []);
  const save = useSaveRole(create);
  const del = useDeleteRole();
  const set = (k: keyof typeof f) => (v: string) => setF((x) => ({ ...x, [k]: v }));
  const holders = role ? team.members.filter((m) => m.roles.includes(role.key)).length : 0;
  const held = role ? team.members.some((m) => m.self && m.roles.includes(role.key)) : false;
  const valid = /^staff\.[a-z][a-z0-9_]{1,30}$/.test(f.key) && f.name.trim() && f.reason.trim().length >= 5;
  return (
    <Drawer isOpen onOpenChange={(o) => !o && onClose()} title={create ? "New role" : role.name}
      subtitle={create ? "Roles are data: pick exactly what this role may do." : `${role.key} · ${holders} member${holders === 1 ? "" : "s"}${role.preset ? " · preset" : ""}`}
      footer={canEdit && (
        <>
          <Button variant="primary" isDisabled={!valid || save.isPending} tooltip={valid ? `Saved and audited, then synced to ${idp}` : "Key, name and a reason are required"}
            onPress={() => save.mutate({ key: f.key, name: f.name.trim(), description: f.description.trim(), permissions: perms, reason: f.reason.trim() }, { onSuccess: onClose })}>
            {create ? "Create role" : "Save role"}
          </Button>
          {!create && (
            <Button variant="danger" className="ml-auto" isDisabled={del.isPending} tooltip={`Remove it here and from every member in ${idp}`}
              onPress={() => del.mutate(role.key, { onSuccess: onClose })}><Trash2 className="size-4" /> Delete</Button>
          )}
        </>
      )}>
      <div className="space-y-4 px-6 py-5">
        {create && <TextField label="Key" mono value={f.key} onChange={set("key")} description={`Role key in ${idp}, e.g. staff.oncall. Can't change later.`} />}
        <TextField label="Name" value={f.name} onChange={set("name")} isDisabled={!canEdit} />
        <TextField label="Description" value={f.description} onChange={set("description")} isDisabled={!canEdit} />
        {held && <p className="text-xs text-warning">You hold this role: you can remove permissions but not add any.</p>}
        <PermissionPicker catalog={team.catalog} value={perms} onChange={setPerms} isDisabled={!canEdit} />
        {canEdit && <TextField label="Reason" multiline value={f.reason} onChange={set("reason")} description="Required; kept in the audit log." />}
      </div>
    </Drawer>
  );
}
