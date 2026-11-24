import { useState, type ReactNode } from "react";
import { Button } from "../../../../../lib/design/components/button/button";
import { ModalDialog } from "../../../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../../../lib/design/components/field/text-field";
import { minReason, reasonOK } from "./model";

type Props = {
  title: string;
  description?: ReactNode;
  children?: ReactNode;
  confirmLabel: string;
  danger?: boolean;
  pending?: boolean;
  /** The reason typed last time, so a retry after a refusal doesn't ask for it again. */
  initialReason?: string;
  onCancel: () => void;
  onConfirm: (reason: string) => void;
};

/** Asks why a change is being made before it is sent. The reason goes to the audit log and the history. */
export function ReasonDialog({
  title,
  description,
  children,
  confirmLabel,
  danger,
  pending,
  initialReason = "",
  onCancel,
  onConfirm,
}: Props) {
  const [reason, setReason] = useState(initialReason);
  const ok = reasonOK(reason);
  return (
    <ModalDialog isOpen onOpenChange={(o) => !o && onCancel()} title={title} description={description}>
      <form
        className="space-y-4 px-6 py-5 text-base text-fg-muted"
        onSubmit={(e) => {
          e.preventDefault();
          if (ok && !pending) onConfirm(reason.trim());
        }}
      >
        {children}
        <TextField
          label="Why are you making this change?"
          value={reason}
          onChange={setReason}
          autoFocus
          placeholder="A few words, such as: setting up the GitHub App"
          description={`Kept in the audit log and this section's history. At least ${minReason} characters.`}
        />
        <div className="flex justify-end gap-2 border-t border-line pt-4">
          <Button variant="ghost" tooltip="Close without changing anything" onPress={onCancel}>
            Cancel
          </Button>
          <Button
            type="submit"
            variant={danger ? "danger" : "primary"}
            isDisabled={!ok || pending}
            tooltip={ok ? confirmLabel : `Give a reason of at least ${minReason} characters first`}
          >
            {pending ? "Working…" : confirmLabel}
          </Button>
        </div>
      </form>
    </ModalDialog>
  );
}
