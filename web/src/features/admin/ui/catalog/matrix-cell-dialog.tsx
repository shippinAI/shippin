import { useState } from "react";
import type { Lock, Plan, PlanService } from "../../../../lib/api/catalog-types";
import { Button } from "../../../../lib/design/components/button/button";
import { ModalDialog } from "../../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { Select } from "../../../../lib/design/components/select/select";
import { Switch } from "../../../../lib/design/components/switch/switch";
import { catalogPaths, useCatalogWrite } from "../../../../lib/hooks/use-catalog";
import { explain } from "./term-tip";

const locks: Lock[] = ["bounded", "overridable", "mergeable", "locked"];

/** Edit one plan x service cell: included, lock mode and limits (JSON object). */
export function MatrixCellDialog({ plan, service, cell, onClose }: { plan: Plan; service: string; cell?: PlanService; onClose: () => void }) {
  const [enabled, setEnabled] = useState(cell?.enabled ?? true);
  const [lock, setLock] = useState<Lock>(cell?.lock ?? "bounded");
  const [limits, setLimits] = useState(JSON.stringify(cell?.limits ?? {}, null, 2));
  const save = useCatalogWrite(`${service} on ${plan.label} saved`);
  const remove = useCatalogWrite(`${service} removed from ${plan.label}`);
  let parsed: unknown = null;
  try { parsed = JSON.parse(limits); } catch { parsed = null; }
  const valid = parsed !== null && typeof parsed === "object" && !Array.isArray(parsed);
  return (
    <ModalDialog isOpen onOpenChange={(o) => !o && onClose()} title={`${service} on ${plan.label}`} description={explain(lock)}>
      <div className="grid gap-4 px-6 py-5">
        <Switch isSelected={enabled} onChange={setEnabled} tooltip="Off keeps the row but disables the service for this tier">Included</Switch>
        <Select label="Lock mode" value={lock} onChange={(v) => setLock(v as Lock)} options={locks.map((l) => ({ id: l, label: l }))} />
        <TextField label="Limits" description={valid ? "Numbers become maximums under bounded." : "Must be a JSON object"} multiline mono value={limits} onChange={setLimits} />
      </div>
      <div className="flex gap-2 border-t border-line px-6 py-4">
        {cell && <Button tooltip="Remove" variant="danger" onPress={() => remove.mutate({ method: "DELETE", path: catalogPaths.cell(plan.id, service) }, { onSuccess: onClose })}>Remove</Button>}
        <div className="flex-1" />
        <Button tooltip="Cancel" variant="ghost" onPress={onClose}>Cancel</Button>
        <Button tooltip="Save" variant="primary" isDisabled={!valid || save.isPending}
          onPress={() => save.mutate({ method: "PUT", path: catalogPaths.cell(plan.id, service), body: { enabled, lock, limits: parsed } }, { onSuccess: onClose })}>Save</Button>
      </div>
    </ModalDialog>
  );
}
