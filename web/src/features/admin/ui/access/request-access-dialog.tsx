import { ShieldQuestion } from "lucide-react";
import { useEffect, useState } from "react";
import { Button } from "../../../../lib/design/components/button/button";
import { ModalDialog } from "../../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { Select } from "../../../../lib/design/components/select/select";
import { useAdminGrants, useCreateGrant } from "../../../../lib/hooks/use-admin-access";
import { durationLabel } from "../../../access/ui/grant-meta";
import { ScopeSelect, type Target } from "./scope-select";

type Props = { target?: Target; onClose: () => void };

/** Ask the customer's owners/admins for time-boxed access (reason, duration, scope required). */
export function RequestAccessDialog({ target, onClose }: Props) {
  const cfg = useAdminGrants().data?.config;
  const create = useCreateGrant("request");
  const [reason, setReason] = useState("");
  const [duration, setDuration] = useState("");
  const [ws, setWs] = useState("");
  useEffect(() => {
    setReason("");
    setWs(target?.workspaceId ?? "");
    setDuration(cfg?.durations[0] ?? "");
  }, [target, cfg]);
  if (!target) return null;
  const n = reason.trim().length;
  const valid = !!cfg && n >= cfg.reason_min && n <= cfg.reason_max && !!duration;
  return (
    <ModalDialog
      isOpen
      onOpenChange={(o) => !o && onClose()}
      title="Request access"
      description={<>The owners and admins of <span className="text-fg">{target.tenantName}</span> see your request with Approve / Deny. Nothing opens until they approve.</>}
    >
      <div className="space-y-4 px-6 py-5">
        <ScopeSelect target={target} value={ws} onChange={setWs} />
        <Select label="Duration" options={(cfg?.durations ?? []).map((d) => ({ id: d, label: durationLabel(d) }))} value={duration} onChange={setDuration} />
        <TextField
          label="Reason"
          multiline
          isRequired
          value={reason}
          onChange={setReason}
          placeholder="e.g. Ticket #4821: the customer asked us to look at a failing build in their workspace."
          description={cfg ? `Shown to the customer and kept in the audit log. ${cfg.reason_min}–${cfg.reason_max} characters (${n}).` : undefined}
        />
        <div className="flex items-center justify-end gap-2 border-t border-line pt-4">
          <Button variant="ghost" tooltip="Close without requesting" onPress={onClose}>Cancel</Button>
          <Button
            variant="primary"
            isDisabled={!valid || create.isPending}
            tooltip={valid ? "Send the request to the customer" : "Add a reason and choose a duration"}
            onPress={() => create.mutate({ tenant_id: target.tenantId, workspace_id: ws || undefined, reason: reason.trim(), duration }, { onSuccess: onClose })}
          >
            <ShieldQuestion className="size-4" /> Send request
          </Button>
        </div>
      </div>
    </ModalDialog>
  );
}
