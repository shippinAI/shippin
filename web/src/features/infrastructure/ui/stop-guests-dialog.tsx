import { useEffect, useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { ModalDialog } from "../../../lib/design/components/dialog/modal-dialog";
import { Switch } from "../../../lib/design/components/switch/switch";
import type { Guest } from "../../../lib/hooks/use-hosts";
import { useStopGuests } from "../../../lib/hooks/use-infra";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { stoppableGuests } from "../model";
import { BoardNotice } from "./board-notice";

type Props = { serverId: string; guests: Guest[]; isOpen: boolean; onOpenChange: (open: boolean) => void };

/** Choose which guests to stop. They stop one at a time, top to bottom, each with its own result. */
export function StopGuestsDialog({ serverId, guests, isOpen, onOpenChange }: Props) {
  const t = useText();
  const stop = useStopGuests(serverId);
  const candidates = stoppableGuests(guests);
  const [chosen, setChosen] = useState<Set<string>>(new Set());
  const [force, setForce] = useState(false);

  useEffect(() => {
    if (!isOpen) return;
    setChosen(new Set(candidates.filter((g) => !g.self).map((g) => g.id)));
    setForce(false);
    stop.reset();
    // Reset only when the dialog opens: the guest list refreshes on a timer behind it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen]);

  const toggle = (id: string, on: boolean) => setChosen((s) => {
    const next = new Set(s);
    if (on) next.add(id); else next.delete(id);
    return next;
  });
  const ordered = candidates.filter((g) => chosen.has(g.id)).map((g) => g.id);
  return (
    <ModalDialog isOpen={isOpen} onOpenChange={onOpenChange} title={t("infra.stop.title")} description={t("infra.stop.description")}>
      <div className="space-y-4 px-6 py-5">
        {candidates.length === 0 && <p className="text-sm text-fg-muted">{t("infra.stop.none")}</p>}
        <ul className="max-h-72 space-y-1 overflow-y-auto" data-testid="stop-list">
          {candidates.map((g) => (
            <li key={g.id}>
              <label className={`flex items-center gap-2 rounded-md px-2 py-1.5 text-sm ${g.self ? "text-fg-subtle" : "text-fg"}`}>
                <input type="checkbox" aria-label={g.name || g.id} checked={chosen.has(g.id)} disabled={g.self} onChange={(e) => toggle(g.id, e.target.checked)} />
                <span className="font-medium">{g.name || g.id}</span>
                <span className="font-mono text-[12px] text-fg-subtle">{g.id}</span>
                {g.self && <span className="ml-auto text-[12px] text-warning">{t("infra.stop.self")}</span>}
              </label>
            </li>
          ))}
        </ul>
        <Switch isSelected={force} onChange={setForce} tooltip={t("infra.stop.forceTooltip")}>{t("infra.stop.force")}</Switch>
        <BoardNotice error={stop.error} retry={() => { if (stop.variables) stop.mutate(stop.variables, { onSuccess: () => onOpenChange(false) }); }} />
        <div className="flex justify-end gap-2 border-t border-line pt-4">
          <Button variant="ghost" tooltip={t("infra.dialog.cancelTooltip")} onPress={() => onOpenChange(false)}>{t("infra.dialog.cancel")}</Button>
          <Button variant="primary" isDisabled={ordered.length === 0 || stop.isPending} tooltip={t("infra.stop.submitTooltip")}
            onPress={() => stop.mutate({ guests: ordered, force }, { onSuccess: () => onOpenChange(false) })}>
            {t("infra.stop.submit", { count: ordered.length })}
          </Button>
        </div>
      </div>
    </ModalDialog>
  );
}
