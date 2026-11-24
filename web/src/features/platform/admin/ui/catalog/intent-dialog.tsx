import { useState } from "react";
import { callAction } from "../../../../../lib/api/actions";
import { Button } from "../../../../../lib/design/components/button/button";
import { ModalDialog } from "../../../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../../../lib/design/components/field/text-field";
import { useCatalogAction } from "../../../../../lib/hooks/use-catalog";

/** Record a provision intent: the customer's effective config as desired state, with an audited reason. */
export function IntentDialog({ customerId, onClose }: { customerId: string; onClose: () => void }) {
  const [note, setNote] = useState("");
  const [reason, setReason] = useState("");
  const reasonOK = reason.trim().length >= 5;
  const record = useCatalogAction("Provision intent recorded (pending change approval)");
  return (
    <ModalDialog
      isOpen
      onOpenChange={(o) => !o && onClose()}
      title="Record provision intent"
      description="Snapshots the effective config as the state to provision. Nothing changes until change control applies it."
    >
      <div className="space-y-4 px-6 py-5">
        <TextField
          label="Reason (audited)"
          value={reason}
          onChange={setReason}
          placeholder="Why this should be provisioned"
          autoFocus
          description="Kept in the audit log with the change. At least 5 characters."
        />
        <TextField label="Note" value={note} onChange={setNote} description="Optional; kept with the intent." />
        <div className="flex justify-end gap-2 border-t border-line pt-4">
          <Button variant="ghost" tooltip="Cancel" onPress={onClose}>
            Cancel
          </Button>
          <Button
            variant="primary"
            isDisabled={!reasonOK || record.isPending}
            tooltip={reasonOK ? "Record the intent" : "Give a reason first"}
            onPress={() =>
              record.mutate(() => callAction("shippin.tenancy.customer.provision_intent.create", { customerId }, { note, reason }), {
                onSuccess: onClose,
              })
            }
          >
            {record.isPending ? "Recording…" : "Record intent"}
          </Button>
        </div>
      </div>
    </ModalDialog>
  );
}
