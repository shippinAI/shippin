import { useState } from "react";
import type { Perm, Team } from "../../../../lib/api/team-types";
import { Button } from "../../../../lib/design/components/button/button";
import { ModalDialog } from "../../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { Select } from "../../../../lib/design/components/select/select";
import { useGrantPerm } from "../../../../lib/hooks/use-perm-grants";

/** Grant one granular permission to one team member (Owner only; the API refuses anyone else). */
export function GrantDialog({ team, perms, onClose }: { team: Team; perms: Perm[]; onClose: () => void }) {
  const grant = useGrantPerm();
  const people = team.members.filter((m) => !m.self && m.state === "active" && m.roles.length > 0);
  const [subject, setSubject] = useState(people[0]?.id ?? "");
  const [permission, setPermission] = useState(perms[0]?.key ?? "");
  const [reason, setReason] = useState("");
  const risky = perms.find((p) => p.key === permission)?.high_risk;
  return (
    <ModalDialog isOpen onOpenChange={(o) => !o && onClose()} title="Grant a permission"
      description="Deny by default: only Owners hold catalog and customer permissions. A grant adds exactly one, for one person, until revoked. It is audited and needs a staff role and MFA to take effect.">
      <div className="grid gap-4 px-6 py-5">
        <Select label="Team member" value={subject} onChange={setSubject}
          options={people.map((m) => ({ id: m.id, label: `${m.name || m.login} · ${m.email}` }))} />
        <Select label="Permission" value={permission} onChange={setPermission}
          options={perms.map((p) => ({ id: p.key, label: `${p.key} — ${p.label}` }))} />
        {risky && <p className="rounded-lg border border-warning/30 bg-warning/10 px-3 py-2 text-[13px] text-warning">High-risk permission: it changes customer data or what Gold provisions.</p>}
        <TextField label="Reason" description="Why this person needs it (5 to 300 characters; kept in the audit log)" value={reason} onChange={setReason} isRequired />
      </div>
      <div className="flex gap-2 border-t border-line px-6 py-4">
        <div className="flex-1" />
        <Button tooltip="Cancel" variant="ghost" onPress={onClose}>Cancel</Button>
        <Button tooltip="Grant and audit" variant="primary" isDisabled={grant.isPending || !subject || !permission || reason.trim().length < 5}
          onPress={() => grant.mutate({ subject, permission, reason: reason.trim() }, { onSuccess: onClose })}>Grant</Button>
      </div>
    </ModalDialog>
  );
}
