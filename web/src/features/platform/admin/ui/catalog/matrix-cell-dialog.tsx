import { useState } from "react";
import { callAction } from "../../../../../lib/api/actions";
import type { Lock, Plan, PlanService } from "../../../../../lib/api/catalog-types";
import { Button } from "../../../../../lib/design/components/button/button";
import { ModalDialog } from "../../../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../../../lib/design/components/field/text-field";
import { Select } from "../../../../../lib/design/components/select/select";
import { Switch } from "../../../../../lib/design/components/switch/switch";
import { useCatalogAction } from "../../../../../lib/hooks/use-catalog";
import { RunnerLimitsFields } from "./runner-limits-fields";
import { runnersService } from "./runner-sizes";
import { explain } from "./term-tip";

const locks: Lock[] = ["bounded", "overridable", "mergeable", "locked"];

/** Edit one plan x service cell: included, lock mode and limits (JSON object). Every change carries a reason for the audit log. */
export function MatrixCellDialog({
  plan,
  service,
  cell,
  onClose,
}: {
  plan: Plan;
  service: string;
  cell?: PlanService;
  onClose: () => void;
}) {
  const [enabled, setEnabled] = useState(cell?.enabled ?? true);
  const [lock, setLock] = useState<Lock>(cell?.lock ?? "bounded");
  const [limits, setLimits] = useState(JSON.stringify(cell?.limits ?? {}, null, 2));
  const [reason, setReason] = useState("");
  const save = useCatalogAction(`${service} on ${plan.label} saved`);
  const remove = useCatalogAction(`${service} removed from ${plan.label}`);
  let parsed: unknown = null;
  try {
    parsed = JSON.parse(limits);
  } catch {
    parsed = null;
  }
  const valid = parsed !== null && typeof parsed === "object" && !Array.isArray(parsed);
  const reasonOK = reason.trim().length >= 5;
  const at = { planId: plan.id, serviceId: service };
  return (
    <ModalDialog isOpen onOpenChange={(o) => !o && onClose()} title={`${service} on ${plan.label}`} description={explain(lock)}>
      <div className="grid gap-4 px-6 py-5">
        <TextField
          label="Reason (audited)"
          value={reason}
          onChange={setReason}
          placeholder="Why this changes"
          description="Kept in the audit log with the change. At least 5 characters."
        />
        <Switch isSelected={enabled} onChange={setEnabled} tooltip="Off keeps the row but disables the service for this tier">
          Included
        </Switch>
        <Select label="Lock mode" value={lock} onChange={(v) => setLock(v as Lock)} options={locks.map((l) => ({ id: l, label: l }))} />
        {service === runnersService && (
          <RunnerLimitsFields
            limits={valid ? (parsed as Record<string, unknown>) : null}
            onChange={(next) => setLimits(JSON.stringify(next, null, 2))}
          />
        )}
        <TextField
          label="Limits"
          description={valid ? "Numbers become maximums under bounded." : "Must be a JSON object"}
          multiline
          mono
          value={limits}
          onChange={setLimits}
        />
      </div>
      <div className="flex gap-2 border-t border-line px-6 py-4">
        {cell && (
          <Button
            tooltip={reasonOK ? "Remove" : "Give a reason first"}
            variant="danger"
            isDisabled={!reasonOK || remove.isPending}
            onPress={() => remove.mutate(() => callAction("shippin.tenancy.plan.service.delete", at, { reason }), { onSuccess: onClose })}
          >
            Remove
          </Button>
        )}
        <div className="flex-1" />
        <Button tooltip="Cancel" variant="ghost" onPress={onClose}>
          Cancel
        </Button>
        <Button
          tooltip={reasonOK ? "Save" : "Give a reason first"}
          variant="primary"
          isDisabled={!valid || !reasonOK || save.isPending}
          onPress={() =>
            save.mutate(() => callAction("shippin.tenancy.plan.service.update", at, { enabled, lock, limits: parsed, reason }), {
              onSuccess: onClose,
            })
          }
        >
          Save
        </Button>
      </div>
    </ModalDialog>
  );
}
