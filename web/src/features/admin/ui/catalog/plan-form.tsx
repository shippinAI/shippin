import { useState } from "react";
import { callAction } from "../../../../lib/api/actions";
import type { Plan } from "../../../../lib/api/catalog-types";
import { Button } from "../../../../lib/design/components/button/button";
import { ModalDialog } from "../../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { Switch } from "../../../../lib/design/components/switch/switch";
import { Select } from "../../../../lib/design/components/select/select";
import { useCatalogAction, useCoverageTiers } from "../../../../lib/hooks/use-catalog";
import { CatalogPerm, useCan } from "../../../../lib/hooks/use-can";

/** What the form edits: a plan's own fields, without what the server keeps (services, updated_at). */
type Fields = Pick<Plan, "id" | "label" | "description" | "rank" | "public" | "active" | "coverage"> & { price_cents: number };

const blank: Fields = { id: "", label: "", description: "", rank: 0, price_cents: 0, public: true, active: true, coverage: "" };

/** Create or edit a plan (tier). Deleting is refused by the API while customers use it. Every change carries a reason for the audit log. */
export function PlanForm({ initial, onClose }: { initial?: Plan; onClose: () => void }) {
  const tiers = useCoverageTiers().data?.items ?? [];
  const [p, setP] = useState<Fields>(initial
    ? { id: initial.id, label: initial.label, description: initial.description, rank: initial.rank, price_cents: initial.price_cents ?? 0,
        public: initial.public, active: initial.active, coverage: initial.coverage }
    : blank);
  const [reason, setReason] = useState("");
  const coverage = p.coverage || tiers.find((t) => t.active)?.id || "";
  const save = useCatalogAction(initial ? "Plan updated" : "Plan created");
  const del = useCatalogAction("Plan deleted");
  const prices = useCan()(CatalogPerm.pricesRead);
  const set = <K extends keyof Fields>(k: K) => (v: Fields[K]) => setP({ ...p, [k]: v });
  // The action takes exactly its input: the id is in the path on an update, and a price only from a caller who may see prices.
  const { id, price_cents, ...fields } = p;
  const input = { ...fields, coverage, ...(prices ? { price_cents } : {}), reason };
  const reasonOK = reason.trim().length >= 5;
  return (
    <ModalDialog isOpen onOpenChange={(o) => !o && onClose()} title={initial ? `Edit ${initial.label}` : "New plan"}
      description="Ids are lowercase and permanent; labels and prices can change any time.">
      <div className="grid grid-cols-2 gap-4 px-6 py-5">
        <div className="col-span-2"><TextField label="Reason (audited)" value={reason} onChange={setReason} placeholder="Why this changes" description="Kept in the audit log with the change. At least 5 characters." /></div>
        <TextField label="Id" value={p.id} onChange={set("id")} isDisabled={!!initial} mono isRequired />
        <TextField label="Label" value={p.label} onChange={set("label")} isRequired />
        <div className="col-span-2"><TextField label="Description" value={p.description} onChange={set("description")} /></div>
        {prices && <TextField label="Price (cents / month)" value={String(p.price_cents)} onChange={(v) => set("price_cents")(Number(v) || 0)} />}
        <TextField label="Sort rank" value={String(p.rank)} onChange={(v) => set("rank")(Number(v) || 0)} />
        <div className="col-span-2"><Select label="Includes coverage" options={tiers.map((t) => ({ id: t.id, label: t.label }))} value={coverage} onChange={set("coverage")} /></div>
        <Switch isSelected={p.public} onChange={set("public")} tooltip="Shown on the public pricing list">Public</Switch>
        <Switch isSelected={p.active} onChange={set("active")} tooltip="Inactive plans can't be picked for new customers">Active</Switch>
      </div>
      <div className="flex gap-2 border-t border-line px-6 py-4">
        {initial && <Button tooltip={reasonOK ? "Delete" : "Give a reason first"} variant="danger" isDisabled={!reasonOK || del.isPending}
          onPress={() => del.mutate(() => callAction("shippin.tenancy.plan.delete", { planId: id }, { reason }), { onSuccess: onClose })}>Delete</Button>}
        <div className="flex-1" />
        <Button tooltip="Cancel" variant="ghost" onPress={onClose}>Cancel</Button>
        <Button tooltip={reasonOK ? "Save plan" : "Give a reason first"} variant="primary" isDisabled={save.isPending || !p.id || !p.label || !reasonOK}
          onPress={() => save.mutate(() => initial
            ? callAction("shippin.tenancy.plan.update", { planId: id }, input)
            : callAction("shippin.tenancy.plan.create", {}, { id, ...input }), { onSuccess: onClose })}>Save plan</Button>
      </div>
    </ModalDialog>
  );
}
