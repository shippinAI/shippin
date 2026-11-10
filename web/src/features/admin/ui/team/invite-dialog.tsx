import { MailPlus } from "lucide-react";
import { useState } from "react";
import { CheckboxGroup, Label } from "react-aria-components";
import { Button } from "../../../../lib/design/components/button/button";
import { Checkbox } from "../../../../lib/design/components/checkbox/checkbox";
import { ModalDialog } from "../../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../../lib/design/components/field/text-field";
import type { StaffRole } from "../../../../lib/api/team-types";
import { useInvite, useInviteLink } from "../../../../lib/hooks/use-team";

/** Add someone. An identity provider that holds staff e-mails them itself; otherwise this gives a link to hand over. */
export function InviteDialog({ roles, local, onClose }: { roles: StaffRole[]; local?: boolean; onClose: () => void }) {
  const invite = useInvite();
  const link = useInviteLink();
  const [f, setF] = useState({ email: "", first_name: "", last_name: "", reason: "" });
  const [picked, setPicked] = useState<string[]>([]);
  const set = (k: keyof typeof f) => (v: string) => setF((x) => ({ ...x, [k]: v }));
  const valid = /^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(f.email.trim()) && f.first_name.trim() && f.last_name.trim() && f.reason.trim().length >= 5;
  return (
    <ModalDialog isOpen onOpenChange={(o) => !o && onClose()} title="Invite a staff member"
      description={local ? "This makes their account. Give them the link it shows, or have them sign in through your identity provider with this e-mail." : "They get an e-mail from the identity provider to set a password, then must enroll MFA before any staff permission applies."}>
      {link.data ? (
        <div className="space-y-4 px-6 py-5" data-testid="invite-link">
          <p className="text-base text-fg-muted">Send them this link. It works once and lasts a week.</p>
          <p className="break-all rounded-lg border border-line bg-canvas p-3 font-mono text-sm text-fg">{link.data.url}</p>
          <div className="flex justify-end gap-2 border-t border-line pt-4">
            <Button variant="secondary" tooltip="Copy the link" onPress={() => void navigator.clipboard?.writeText(link.data.url)}>Copy</Button>
            <Button variant="primary" tooltip="Close" onPress={onClose}>Done</Button>
          </div>
        </div>
      ) : (
      <div className="max-h-[70vh] space-y-4 overflow-y-auto px-6 py-5">
        <TextField label="E-mail" type="email" isRequired value={f.email} onChange={set("email")} placeholder="name@company.com" autoFocus />
        <div className="grid grid-cols-2 gap-3">
          <TextField label="First name" isRequired value={f.first_name} onChange={set("first_name")} />
          <TextField label="Last name" isRequired value={f.last_name} onChange={set("last_name")} />
        </div>
        <CheckboxGroup value={picked} onChange={setPicked} className="space-y-0.5">
          <Label className="text-sm font-medium text-fg">Roles (optional; you can add them later)</Label>
          {roles.map((r) => (
            <Checkbox key={r.key} value={r.key} description={r.description || r.key}>{r.name}</Checkbox>
          ))}
        </CheckboxGroup>
        <TextField label="Reason" multiline value={f.reason} onChange={set("reason")} placeholder="e.g. New support hire, starts Monday" description="Required; kept in the audit log." />
        <div className="flex justify-end gap-2 border-t border-line pt-4">
          <Button variant="ghost" tooltip="Close without inviting" onPress={onClose}>Cancel</Button>
          <Button variant="primary" isDisabled={!valid || invite.isPending} tooltip={valid ? "Create the account and send the e-mail" : "Fill in e-mail, names and a reason"}
            onPress={() => invite.mutate({ email: f.email.trim(), first_name: f.first_name.trim(), last_name: f.last_name.trim(), roles: picked, reason: f.reason.trim() }, { onSuccess: (d) => { if (local && d?.id) link.mutate(d.id); else onClose(); } })}>
            <MailPlus className="size-4" /> {local ? "Add" : "Send invite"}
          </Button>
        </div>
      </div>
      )}
    </ModalDialog>
  );
}
