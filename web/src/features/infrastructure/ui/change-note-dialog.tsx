import { useEffect, useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { ModalDialog } from "../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../lib/design/components/field/text-field";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { noteOK } from "../model";

export type PendingChange = {
  title: string;
  description?: string;
  /** Said plainly before the note, when this change can do damage. */
  warning?: string;
  confirm: string;
  run: (note: string) => void;
};

/** Captures the operator's reason before the API evaluates a provider change request. */
export function ChangeNoteDialog({ change, onClose }: { change: PendingChange | null; onClose: () => void }) {
  const t = useText();
  const [note, setNote] = useState("");
  useEffect(() => {
    if (change) setNote("");
  }, [change]);
  const submit = () => {
    if (!change || !noteOK(note)) return;
    change.run(note.trim());
    onClose();
  };
  return (
    <ModalDialog isOpen={change !== null} onOpenChange={(o) => { if (!o) onClose(); }} title={change?.title ?? ""} description={change?.description}>
      <form className="space-y-4 px-6 py-5" data-testid="change-note" onSubmit={(e) => { e.preventDefault(); submit(); }}>
        {change?.warning && <p role="alert" className="rounded-lg border border-warning/30 bg-warning/10 px-3 py-2 text-[13px] text-warning">{change.warning}</p>}
        <TextField label={t("infra.note.label")} description={t("infra.note.help")} value={note} onChange={setNote} autoFocus />
        <div className="flex justify-end gap-2 border-t border-line pt-4">
          <Button variant="ghost" tooltip={t("infra.dialog.cancelTooltip")} onPress={onClose}>{t("infra.dialog.cancel")}</Button>
          <Button type="submit" variant="primary" isDisabled={!noteOK(note)} tooltip={t("infra.note.tooltip")}>{change?.confirm}</Button>
        </div>
      </form>
    </ModalDialog>
  );
}
