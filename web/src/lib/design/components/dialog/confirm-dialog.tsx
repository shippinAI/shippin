import { useState, type ReactNode } from "react";
import { Button } from "../button/button";
import { TextField } from "../field/text-field";
import { ModalDialog } from "./modal-dialog";

type Props = {
  isOpen: boolean;
  onOpenChange: (o: boolean) => void;
  title: string;
  body: ReactNode;
  confirmLabel: string;
  /** When set, the user must type this text to confirm. */
  typeToConfirm?: string;
  onConfirm: () => void;
};

/** Destructive confirmation with optional typed-name check. */
export function ConfirmDialog({ isOpen, onOpenChange, title, body, confirmLabel, typeToConfirm, onConfirm }: Props) {
  const [typed, setTyped] = useState("");
  const ok = !typeToConfirm || typed === typeToConfirm;
  return (
    <ModalDialog isOpen={isOpen} onOpenChange={onOpenChange} title={title}>
      <div className="space-y-4 px-6 py-5 text-base text-fg-muted">
        {body}
        {typeToConfirm && (
          <TextField label={`Type ${typeToConfirm} to confirm`} value={typed} onChange={setTyped} autoFocus />
        )}
        <div className="flex justify-end gap-2 border-t border-line pt-4">
          <Button variant="ghost" tooltip="Keep it" onPress={() => onOpenChange(false)}>Cancel</Button>
          <Button
            variant="danger"
            isDisabled={!ok}
            tooltip={ok ? "This can't be undone" : `Type ${typeToConfirm} first`}
            onPress={() => {
              onConfirm();
              onOpenChange(false);
              setTyped("");
            }}
          >
            {confirmLabel}
          </Button>
        </div>
      </div>
    </ModalDialog>
  );
}
