import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { callAction } from "../../../../lib/api/actions";
import type { CoverageTier } from "../../../../lib/api/catalog-types";
import { Button } from "../../../../lib/design/components/button/button";
import { Checkbox } from "../../../../lib/design/components/checkbox/checkbox";
import { Drawer } from "../../../../lib/design/components/drawer/drawer";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { Select } from "../../../../lib/design/components/select/select";
import { Switch } from "../../../../lib/design/components/switch/switch";
import { toast } from "../../../../lib/design/components/toast/toast-queue";
import { CatalogPerm, useCan } from "../../../../lib/hooks/use-can";
import { channelLabel, operationsLabel, reviewsLabel, services, severities, signInLabel, supportHoursLabel, termLabel } from "../../../coverage/wording";

const blank: CoverageTier = {
  id: "", label: "", description: "", rank: 0, sign_in: "password", support_hours: "business", channels: ["docs", "email"],
  critical_minutes: null, high_minutes: null, normal_minutes: null, uptime_target: null, operations: "self", named_contact: false,
  reviews: "none", training_hours: 0, consulting: "quoted", custom_deployments: "quoted", remote_operations: "none", team_training: "quoted",
  price_cents: 0, spend_percent: 0, public: true, active: true, updated_at: "",
};

const options = <K extends string>(labels: Record<K, string>) => (Object.keys(labels) as K[]).map((id) => ({ id, label: labels[id] }));
const optionalNumber = (v: string) => (v.trim() === "" ? null : Number(v));

/** Create or edit a coverage tier. Deleting is refused by the API while a plan includes it. */
export function CoverageForm({ initial, onClose }: { initial?: CoverageTier; onClose: () => void }) {
  const [c, setC] = useState<CoverageTier>(initial ?? blank);
  const [reason, setReason] = useState("");
  const qc = useQueryClient();
  const prices = useCan()(CatalogPerm.pricesRead);
  const set = <K extends keyof CoverageTier>(k: K) => (v: CoverageTier[K]) => setC({ ...c, [k]: v });
  const channel = (ch: CoverageTier["channels"][number], on: boolean) => set("channels")(on ? [...c.channels, ch] : c.channels.filter((x) => x !== ch));
  // The action takes exactly its input fields: the id is in the path on an update, and read-only fields are never sent.
  const { id, updated_at: _read, price_cents, spend_percent, ...fields } = c;
  const input = { ...fields, ...(prices ? { price_cents, spend_percent } : {}), reason };
  const done = (msg: string) => ({ onSuccess: () => { toast.success(msg); void qc.invalidateQueries({ queryKey: ["catalog"] }); onClose(); }, onError: (e: Error) => toast.error("Not saved", e.message) });
  const save = useMutation({ mutationFn: () => initial
    ? callAction("shippin.tenancy.coverage_tier.update", { coverageTierId: id }, input)
    : callAction("shippin.tenancy.coverage_tier.create", {}, { id, ...input }), ...done(initial ? "Coverage updated" : "Coverage tier created") });
  const del = useMutation({ mutationFn: () => callAction("shippin.tenancy.coverage_tier.delete", { coverageTierId: id }, { reason }), ...done("Coverage tier deleted") });
  const reasonOK = reason.trim().length >= 5;
  return (
    <Drawer isOpen onOpenChange={(o) => !o && onClose()} title={initial ? `Edit ${initial.label}` : "New coverage tier"}
      subtitle="Every value is yours to set. Response targets are minutes to a first reply, counted within the support hours."
      footer={<>
        {initial && <Button tooltip={reasonOK ? "Delete this tier" : "Give a reason first"} variant="danger" isDisabled={!reasonOK || del.isPending} onPress={() => del.mutate()}>Delete</Button>}
        <div className="flex-1" />
        <Button tooltip="Cancel" variant="ghost" onPress={onClose}>Cancel</Button>
        <Button tooltip={reasonOK ? "Save coverage tier" : "Give a reason first"} variant="primary" isDisabled={save.isPending || !c.id || !c.label || !reasonOK}
          onPress={() => save.mutate()}>Save</Button>
      </>}>
      <div className="grid grid-cols-2 gap-4 px-6 py-5">
        <div className="col-span-2"><TextField label="Reason (audited)" value={reason} onChange={setReason} placeholder="Why this changes" description="Kept in the audit log with the change. At least 5 characters." /></div>
        <TextField label="Id" value={c.id} onChange={set("id")} isDisabled={!!initial} mono isRequired />
        <TextField label="Label" value={c.label} onChange={set("label")} isRequired />
        <div className="col-span-2"><TextField label="Description" multiline value={c.description} onChange={set("description")} /></div>
        <TextField label="Sort rank" value={String(c.rank)} onChange={(v) => set("rank")(Number(v) || 0)} />
        <Select label="Members sign in with" options={options(signInLabel)} value={c.sign_in} onChange={(v) => set("sign_in")(v as CoverageTier["sign_in"])} />

        <h3 className="col-span-2 mt-2 text-sm font-semibold text-fg">Support</h3>
        <div className="col-span-2"><Select label="Support hours" options={options(supportHoursLabel)} value={c.support_hours} onChange={(v) => set("support_hours")(v as CoverageTier["support_hours"])} /></div>
        {severities.map((s) => (
          <TextField key={s.key} label={`${s.label}: first reply (minutes)`} description={`${s.hint}. Empty for no target.`}
            value={c[s.key] == null ? "" : String(c[s.key])} onChange={(v) => set(s.key)(optionalNumber(v))} />
        ))}
        <TextField label="Uptime target (%)" description="Such as 99.9. Empty for no commitment."
          value={c.uptime_target == null ? "" : String(c.uptime_target)} onChange={(v) => set("uptime_target")(optionalNumber(v))} />
        <fieldset className="col-span-2">
          <legend className="mb-1 text-sm text-fg-muted">Channels</legend>
          <div className="grid grid-cols-3 gap-1">
            {(Object.keys(channelLabel) as CoverageTier["channels"]).map((ch) => (
              <Checkbox key={ch} isSelected={c.channels.includes(ch)} onChange={(on) => channel(ch, on)}>{channelLabel[ch]}</Checkbox>
            ))}
          </div>
        </fieldset>

        <h3 className="col-span-2 mt-2 text-sm font-semibold text-fg">Operations and services</h3>
        <div className="col-span-2"><Select label="Who operates it" options={options(operationsLabel)} value={c.operations} onChange={(v) => set("operations")(v as CoverageTier["operations"])} /></div>
        <Select label="Reviews" options={options(reviewsLabel)} value={c.reviews} onChange={(v) => set("reviews")(v as CoverageTier["reviews"])} />
        <TextField label="Training included (hours a year)" value={String(c.training_hours)} onChange={(v) => set("training_hours")(Number(v) || 0)} />
        {services.map((s) => (
          <Select key={s.key} label={s.label} options={options(termLabel)} value={c[s.key]} onChange={(v) => set(s.key)(v as CoverageTier["consulting"])} />
        ))}
        <div className="col-span-2"><Switch isSelected={c.named_contact} onChange={set("named_contact")} tooltip="One person on your side who knows this customer">Named contact</Switch></div>

        {prices && <>
          <h3 className="col-span-2 mt-2 text-sm font-semibold text-fg">Price</h3>
          <TextField label="Monthly minimum (cents)" value={String(c.price_cents ?? 0)} onChange={(v) => set("price_cents")(Number(v) || 0)} />
          <TextField label="Share of plan charges (%)" description="The charge is whichever is greater." value={String(c.spend_percent ?? 0)} onChange={(v) => set("spend_percent")(Number(v) || 0)} />
        </>}
        <Switch isSelected={c.public} onChange={set("public")} tooltip="Shown on the public pricing list">Public</Switch>
        <Switch isSelected={c.active} onChange={set("active")} tooltip="Inactive tiers can't be picked for plans or customers">Active</Switch>
      </div>
    </Drawer>
  );
}
