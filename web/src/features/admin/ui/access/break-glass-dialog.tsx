import { Siren } from "lucide-react";
import { useEffect, useState } from "react";
import { Button } from "../../../../lib/design/components/button/button";
import { ModalDialog } from "../../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { Select } from "../../../../lib/design/components/select/select";
import { Switch } from "../../../../lib/design/components/switch/switch";
import { useAdminGrants, useCreateGrant } from "../../../../lib/hooks/use-admin-access";
import { ScopeSelect, type Target } from "./scope-select";

/** Emergency access without customer approval: category + justification + fresh sign-in. */
export function BreakGlassDialog({ target, onClose }: { target?: Target; onClose: () => void }) {
  const bg = useAdminGrants().data?.config.break_glass;
  const create = useCreateGrant("breakglass");
  const [cat, setCat] = useState("");
  const [why, setWhy] = useState("");
  const [ws, setWs] = useState("");
  const [withhold, setWithhold] = useState(false);
  useEffect(() => {
    setWhy("");
    setWithhold(false);
    setWs(target?.workspaceId ?? "");
    setCat(bg?.categories[0]?.id ?? "");
  }, [target, bg]);
  if (!target || !bg) return null;
  const category = bg.categories.find((c) => c.id === cat);
  const valid = !!category && why.trim().length >= bg.justification_min;
  return (
    <ModalDialog
      isOpen
      onOpenChange={(o) => !o && onClose()}
      title="Break glass"
      description={<>Immediate {bg.duration} access to <span className="text-fg">{target.tenantName}</span> without the customer's approval. For emergencies only: it's logged in red, and the customer is told.</>}
    >
      <div className="space-y-4 px-6 py-5">
        <div className="flex gap-3 rounded-lg border border-danger/35 bg-danger/10 px-3.5 py-3 text-[12.5px] text-fg-muted">
          <Siren className="mt-0.5 size-4 shrink-0 text-danger" />
          <span>You'll be asked to sign in again if you haven't in the last {bg.reauth_minutes} minutes. Every open and action while active is audited as break-glass.</span>
        </div>
        <Select label="Category" options={bg.categories.map((c) => ({ id: c.id, label: c.label }))} value={cat} onChange={(v) => { setCat(v); setWithhold(false); }} />
        <ScopeSelect target={target} value={ws} onChange={setWs} />
        <TextField
          label="Justification"
          multiline
          isRequired
          value={why}
          onChange={setWhy}
          placeholder="What's happening, which ticket or report, and why customer approval can't wait."
          description={`At least ${bg.justification_min} characters (${why.trim().length}). Shown to the customer.`}
        />
        {category?.withhold_notice && (
          <Switch isSelected={withhold} onChange={setWithhold} tooltip={`Delays the customer notice by ${category.notice_delay}; the delay itself is audited`}>
            Delay the customer notice ({category.notice_delay}) — legal hold only
          </Switch>
        )}
        <div className="flex items-center justify-end gap-2 border-t border-line pt-4">
          <Button variant="ghost" tooltip="Close without activating" onPress={onClose}>Cancel</Button>
          <Button
            variant="danger"
            isDisabled={!valid || create.isPending}
            tooltip={valid ? `Activate ${bg.duration} of audited emergency access` : "Choose a category and write a justification"}
            onPress={() =>
              create.mutate(
                { tenant_id: target.tenantId, workspace_id: ws || undefined, category: cat, justification: why.trim(), withhold_notice: withhold || undefined },
                { onSuccess: onClose },
              )
            }
          >
            <Siren className="size-4" /> Activate break-glass
          </Button>
        </div>
      </div>
    </ModalDialog>
  );
}
