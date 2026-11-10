import { useEffect, useState } from "react";
import { Button } from "../../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../../lib/design/components/card/card";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { Select } from "../../../../lib/design/components/select/select";
import { Switch } from "../../../../lib/design/components/switch/switch";
import type { Team, TeamSettings } from "../../../../lib/api/team-types";
import { useSaveSettings } from "../../../../lib/hooks/use-team";
import { useIdpName } from "../../../../lib/hooks/use-idp-name";

const modes = [
  { id: "legacy", label: "Legacy — only the IAM owner gate (rollback)" },
  { id: "shadow", label: "Shadow — legacy decides, staff roles logged" },
  { id: "enforce", label: "Enforce — staff roles decide" },
];

/** Team settings stored in the database; every save is audited with a reason. */
export function SettingsPanel({ team, canEdit }: { team: Team; canEdit: boolean }) {
  const idp = useIdpName();
  const [s, setS] = useState<TeamSettings>(team.settings);
  const [reason, setReason] = useState("");
  const save = useSaveSettings();
  useEffect(() => setS(team.settings), [team.settings]);
  const set = <K extends keyof TeamSettings>(k: K) => (v: TeamSettings[K]) => setS((x) => ({ ...x, [k]: v }));
  const num = (k: "idle_days" | "approval_minutes") => (v: string) => set(k)(Number(v.replace(/\D/g, "")) || 0);
  const dirty = JSON.stringify(s) !== JSON.stringify(team.settings);
  const row = "flex items-start justify-between gap-6 border-b border-line px-5 py-4 last:border-b-0";
  const help = "mt-0.5 text-sm text-fg-subtle";
  return (
    <Card className="overflow-hidden">
      <CardHeader title="Team settings" description={team.mode_override ? `Mode is pinned to "${team.mode_override}" by config (staff.mode); the stored mode applies when that is cleared.` : "Stored in the database; changes apply within seconds."} />
      <div className={row}><div><div className="text-base text-fg">Authorization mode</div><div className={help}>Roll forward or back without a deploy.</div></div>
        <div className="w-96"><Select label="Mode" options={modes} value={s.mode} onChange={(v) => set("mode")(v as TeamSettings["mode"])} /></div></div>
      <div className={row}><div><div className="text-base text-fg">IAM owner fallback</div><div className={help}>In enforce mode, {idp} instance owners keep Owner rights. Every use is audited.</div></div>
        <Switch isSelected={s.legacy_fallback} onChange={set("legacy_fallback")} isDisabled={!canEdit} tooltip="authz.legacy_iam_owner is logged on use" /></div>
      {team.local_roles && (
        <div className={row}><div><div className="text-base text-fg">Staff need a second factor</div><div className={help}>Roles apply once someone has a passkey or signs in through an identity provider. Browsers refuse passkeys on a site with a certificate warning, so turn this off only if yours has one and you have no provider.</div></div>
          <Switch isSelected={!s.no_second_factor} onChange={(on) => set("no_second_factor")(!on)} isDisabled={!canEdit} tooltip="Off lets roles apply with a recovery code alone" /></div>
      )}
      <div className={row}><div><div className="text-base text-fg">Two-person rule</div><div className={help}>High-risk grants and role edits wait for a second Owner. Takes effect with 2+ Owners ({team.owners} now{team.two_person_active ? ", active" : ""}).</div></div>
        <Switch isSelected={s.two_person} onChange={set("two_person")} isDisabled={!canEdit} tooltip="Requester can never approve their own change" /></div>
      <div className={row}><div><div className="text-base text-fg">Break-glass needs an approver</div><div className={help}>Someone else with access.approve_breakglass approves. Never self-approval.</div></div>
        <Switch isSelected={s.breakglass_approval} onChange={set("breakglass_approval")} isDisabled={!canEdit} tooltip="Off: break-glass is immediate (still audited red)" /></div>
      <div className={row}><div><div className="text-base text-fg">Sole-Owner activation</div><div className={help}>When nobody else can approve, an Owner may activate alone; flagged red for review.</div></div>
        <Switch isSelected={s.sole_owner_activation} onChange={set("sole_owner_activation")} isDisabled={!canEdit} tooltip="Audited as breakglass.unapproved" /></div>
      <div className={row}><div><div className="text-base text-fg">Timeouts</div><div className={help}>Idle staff are flagged; pending break-glass lapses.</div></div>
        <div className="grid w-96 grid-cols-2 gap-3">
          <TextField label="Idle after (days)" value={String(s.idle_days)} onChange={num("idle_days")} isDisabled={!canEdit} />
          <TextField label="Approval window (min)" value={String(s.approval_minutes)} onChange={num("approval_minutes")} isDisabled={!canEdit} />
        </div></div>
      {canEdit && (
        <div className="flex items-end gap-3 bg-canvas/30 px-5 py-4">
          <div className="flex-1"><TextField label="Reason" value={reason} onChange={setReason} placeholder="Why this change" /></div>
          <Button variant="ghost" isDisabled={!dirty} tooltip="Discard edits" onPress={() => setS(team.settings)}>Reset</Button>
          <Button variant="primary" isDisabled={!dirty || reason.trim().length < 5 || save.isPending} tooltip={dirty ? "Save and audit" : "Nothing changed"}
            onPress={() => save.mutate({ ...s, reason: reason.trim() }, { onSuccess: () => setReason("") })}>Save settings</Button>
        </div>
      )}
    </Card>
  );
}
