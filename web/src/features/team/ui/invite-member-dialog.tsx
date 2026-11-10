import { MailPlus } from "lucide-react";
import { useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { ModalDialog } from "../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../lib/design/components/field/text-field";
import { Select } from "../../../lib/design/components/select/select";
import type { TenantTeam } from "../../../lib/api/tenant-team-types";
import { useTeamWrites } from "../../../lib/hooks/use-tenant-team";
import { useIdpName } from "../../../lib/hooks/use-idp-name";

/** Invite by e-mail: an existing account joins the org; a new one gets a sign-up e-mail. */
export function InviteMemberDialog({ tenantId, team, onClose }: { tenantId: string; team: TenantTeam; onClose: () => void }) {
  const idp = useIdpName();
  const { invite } = useTeamWrites(tenantId);
  const roles = team.roles.filter((r) => team.is_owner || r !== team.owner_role);
  const [f, setF] = useState({ email: "", first_name: "", last_name: "", role: roles[roles.length - 1] ?? "" });
  const set = (k: keyof typeof f) => (v: string) => setF((x) => ({ ...x, [k]: v }));
  const valid = /^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(f.email.trim()) && f.role !== "";
  return (
    <ModalDialog isOpen onOpenChange={(o) => !o && onClose()} title="Invite a member"
      description={`They join this tenant's organization in ${idp}. Someone without an account gets an e-mail to set one up.`}>
      <div className="space-y-4 px-6 py-5">
        <TextField label="E-mail" type="email" isRequired value={f.email} onChange={set("email")} placeholder="name@company.com" autoFocus />
        <div className="grid grid-cols-2 gap-3">
          <TextField label="First name" value={f.first_name} onChange={set("first_name")} description="Needed for new accounts" />
          <TextField label="Last name" value={f.last_name} onChange={set("last_name")} />
        </div>
        <Select label="Role" options={roles.map((r) => ({ id: r, label: r }))} value={f.role} onChange={set("role")} />
        <div className="flex justify-end gap-2 border-t border-line pt-4">
          <Button variant="ghost" tooltip="Close without inviting" onPress={onClose}>Cancel</Button>
          <Button variant="primary" isDisabled={!valid || invite.isPending} tooltip={valid ? "Send the invite (audited)" : "Enter an e-mail and pick a role"}
            onPress={() => invite.mutate({ email: f.email.trim(), first_name: f.first_name.trim(), last_name: f.last_name.trim(), roles: [f.role] }, { onSuccess: onClose })}>
            <MailPlus className="size-4" /> Send invite
          </Button>
        </div>
      </div>
    </ModalDialog>
  );
}
