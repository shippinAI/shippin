import { useState } from "react";
import type { Plan } from "../../../../lib/api/catalog-types";
import { Button } from "../../../../lib/design/components/button/button";
import { ModalDialog } from "../../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { Switch } from "../../../../lib/design/components/switch/switch";
import { catalogPaths, useCatalogWrite } from "../../../../lib/hooks/use-catalog";
import { CatalogPerm, useCan } from "../../../../lib/hooks/use-can";

const blank: Plan = { id: "", label: "", description: "", rank: 0, price_cents: 0, public: true, active: true };

/** Create or edit a plan (tier). Deleting is refused by the API while customers use it. */
export function PlanForm({ initial, onClose }: { initial?: Plan; onClose: () => void }) {
  const [p, setP] = useState<Plan>(initial ?? blank);
  const save = useCatalogWrite(initial ? "Plan updated" : "Plan created");
  const del = useCatalogWrite("Plan deleted");
  const prices = useCan()(CatalogPerm.pricesRead);
  const set = <K extends keyof Plan>(k: K) => (v: Plan[K]) => setP({ ...p, [k]: v });
  return (
    <ModalDialog isOpen onOpenChange={(o) => !o && onClose()} title={initial ? `Edit ${initial.label}` : "New plan"}
      description="Ids are lowercase and permanent; labels and prices can change any time.">
      <div className="grid grid-cols-2 gap-4 px-6 py-5">
        <TextField label="Id" value={p.id} onChange={set("id")} isDisabled={!!initial} mono isRequired />
        <TextField label="Label" value={p.label} onChange={set("label")} isRequired />
        <div className="col-span-2"><TextField label="Description" value={p.description} onChange={set("description")} /></div>
        {prices && <TextField label="Price (cents / month)" value={String(p.price_cents)} onChange={(v) => set("price_cents")(Number(v) || 0)} />}
        <TextField label="Sort rank" value={String(p.rank)} onChange={(v) => set("rank")(Number(v) || 0)} />
        <Switch isSelected={p.public} onChange={set("public")} tooltip="Shown on the public pricing list">Public</Switch>
        <Switch isSelected={p.active} onChange={set("active")} tooltip="Inactive plans can't be picked for new customers">Active</Switch>
      </div>
      <div className="flex gap-2 border-t border-line px-6 py-4">
        {initial && <Button tooltip="Delete" variant="danger" onPress={() => del.mutate({ method: "DELETE", path: catalogPaths.plan(p.id) }, { onSuccess: onClose })}>Delete</Button>}
        <div className="flex-1" />
        <Button tooltip="Cancel" variant="ghost" onPress={onClose}>Cancel</Button>
        <Button tooltip="Save plan" variant="primary" isDisabled={save.isPending || !p.id || !p.label}
          onPress={() => save.mutate({ method: initial ? "PUT" : "POST", path: catalogPaths.plan(initial?.id), body: prices ? p : { ...p, price_cents: undefined } }, { onSuccess: onClose })}>Save plan</Button>
      </div>
    </ModalDialog>
  );
}
