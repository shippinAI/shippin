import { useState } from "react";
import { apiSend } from "../../../lib/api/client";
import { Button } from "../../../lib/design/components/button/button";
import { Checkbox } from "../../../lib/design/components/checkbox/checkbox";
import { ModalDialog } from "../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../lib/design/components/field/text-field";
import { useMe } from "../../../lib/hooks/use-api";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { noteOK } from "../model";
import { PowerReviewDialog } from "./power-review-dialog";

type Verb = "start" | "stop" | "shutdown" | "reboot";
type Props = { base: string; guest: string; verb: Verb; confirmSelf?: boolean; onClose: () => void; onExecuted?: () => void };

/** Asks for a reason and runs the action, or sends it for review when this installation has connected a reviewer. */
export function GuestPowerDialog(props: Props) {
  const reviewed = useMe().data?.capabilities["feature.external_review"] === true;
  return reviewed ? <PowerReviewDialog {...props} /> : <DirectPowerDialog {...props} />;
}

function DirectPowerDialog({ base, guest, verb, confirmSelf = false, onClose, onExecuted }: Props) {
  const t = useText();
  const [note, setNote] = useState("");
  const [understood, setUnderstood] = useState(false);
  const [error, setError] = useState("");
  const [running, setRunning] = useState(false);
  const ready = noteOK(note) && (!confirmSelf || understood) && !running;
  const run = async () => {
    setError("");
    setRunning(true);
    try {
      await apiSend("POST", `${base}/guests/${encodeURIComponent(guest)}/status/${verb}${confirmSelf ? "?confirm=self" : ""}`, {}, { note: note.trim() });
      onExecuted?.();
      onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("infra.power.failed"));
    } finally {
      setRunning(false);
    }
  };
  return (
    <ModalDialog isOpen onOpenChange={(open) => { if (!open) onClose(); }} title={t("infra.power.title", { verb, guest })} description={t("infra.power.description")}>
      <form className="space-y-4 px-6 py-5" onSubmit={(e) => { e.preventDefault(); if (ready) void run(); }}>
        {confirmSelf && <p role="alert" className="rounded-lg border border-warning/30 bg-warning/10 px-3 py-2 text-sm text-warning">{t("infra.self.body")}</p>}
        <TextField label={t("infra.note.label")} description={t("infra.note.help")} value={note} onChange={setNote} autoFocus />
        {confirmSelf && <Checkbox isSelected={understood} onChange={setUnderstood}>{t("infra.self.understand")}</Checkbox>}
        {error && <p role="alert" className="text-base text-danger">{error}</p>}
        <div className="flex justify-end gap-2 border-t border-line pt-4">
          <Button variant="ghost" tooltip={t("infra.dialog.cancelTooltip")} onPress={onClose}>{t("infra.dialog.cancel")}</Button>
          <Button type="submit" variant="primary" isDisabled={!ready} tooltip={t("infra.note.tooltip")}>{running ? t("infra.power.running") : verb}</Button>
        </div>
      </form>
    </ModalDialog>
  );
}
