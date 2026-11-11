import { useEffect, useState, type ReactNode } from "react";
import { Button } from "../button/button";
import { Checkbox } from "../checkbox/checkbox";
import { TextField } from "../field/text-field";
import { ModalDialog } from "./modal-dialog";

type Props = {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  description?: ReactNode;
  /** The consequence, said plainly before anything can be confirmed. */
  warning: ReactNode;
  /** What the person must tick, in full: "I understand this is my …". */
  understand: string;
  confirm: string;
  busy?: boolean;
  ready?: boolean;
  children?: ReactNode;
  onConfirm: (reason: string) => void;
};

/** A change that cannot be taken back: it states the consequence, and needs a reason and an explicit tick. */
export function StrictDialog({ isOpen, onClose, title, description, warning, understand, confirm, busy = false, ready = true, children, onConfirm }: Props) {
  const [reason, setReason] = useState("");
  const [understood, setUnderstood] = useState(false);
  useEffect(() => {
    if (isOpen) { setReason(""); setUnderstood(false); }
  }, [isOpen]);
  const reasonOK = reason.trim().length >= 5;
  const go = reasonOK && understood && ready && !busy;
  return (
    <ModalDialog isOpen={isOpen} onOpenChange={(open) => { if (!open) onClose(); }} title={title} description={description}>
      <form className="space-y-4 px-6 py-5" data-testid="strict-dialog" onSubmit={(e) => { e.preventDefault(); if (go) onConfirm(reason.trim()); }}>
        <p role="alert" className="rounded-lg border border-warning/30 bg-warning/10 px-3 py-2 text-sm text-warning">{warning}</p>
        {children}
        <TextField label="Why are you doing this?" description="Recorded in the audit log." value={reason} onChange={setReason} />
        <Checkbox isSelected={understood} onChange={setUnderstood}>{understand}</Checkbox>
        <div className="flex justify-end gap-2 border-t border-line pt-4">
          <Button variant="ghost" tooltip="Leave this unchanged" onPress={onClose}>Cancel</Button>
          <Button type="submit" variant="danger" isDisabled={!go} tooltip={go ? confirm : "Give a reason and tick the box first"}>{confirm}</Button>
        </div>
      </form>
    </ModalDialog>
  );
}
