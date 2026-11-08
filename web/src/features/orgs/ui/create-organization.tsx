import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Form } from "react-aria-components";
import { apiSend } from "../../../lib/api/client";
import { Button } from "../../../lib/design/components/button/button";
import { ModalDialog } from "../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../lib/design/components/field/text-field";
import { Select } from "../../../lib/design/components/select/select";
import { useMe } from "../../../lib/hooks/use-api";
import { usePlans } from "../../../lib/hooks/use-catalog";
import { useTeam } from "../../../lib/hooks/use-team";
import { useText } from "../../../lib/ui-config/use-ui-config";

export function CreateOrganization() {
  const me = useMe().data;
  const plans = usePlans();
  const team = useTeam();
  const text = useText();
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [plan, setPlan] = useState("");
  const [owner, setOwner] = useState("");
  const [mode, setMode] = useState("self");
  const selectedPlan = plan || plans.data?.items.find((p) => p.active)?.id || "";
  const mine = me?.user_id.startsWith("local:") ? me.user_id.slice(6) : "";
  const save = useMutation({
    mutationFn: () => apiSend("POST", "/v1/tenants", { name, plan_id: selectedPlan, owner_user_id: mode === "self" && mine ? mine : owner }),
    onSuccess: () => {
      setOpen(false); setName("");
      void qc.invalidateQueries({ queryKey: ["me"] });
      void qc.invalidateQueries({ queryKey: ["orgs"] });
      void qc.invalidateQueries({ queryKey: ["admin", "tenants"] });
      void qc.invalidateQueries({ queryKey: ["catalog", "customers"] });
    },
  });
  if (!me?.capabilities["tenants.create"]) return null;
  return <>
    <Button variant="primary" tooltip={text("organization.create")} onPress={() => { save.reset(); setOpen(true); }}>{text("organization.create")}</Button>
    <ModalDialog isOpen={open} onOpenChange={setOpen} title={text("organization.create")} description={text("organization.createDescription")}>
      <Form className="space-y-4 p-6" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
        <TextField label={text("organization.name")} value={name} onChange={setName} isRequired maxLength={120} autoFocus />
        <Select label={text("organization.plan")} value={selectedPlan} onChange={setPlan} options={(plans.data?.items ?? []).filter((p) => p.active).map((p) => ({ id: p.id, label: p.label }))} />
        <Select label={text("organization.owner")} value={mine ? mode : "other"} onChange={setMode} options={[...(mine ? [{ id: "self", label: text("organization.self") }] : []), { id: "other", label: text("organization.other") }]} />
        {(mode === "other" || !mine) && <Select label={text("organization.ownerId")} value={owner} onChange={setOwner} options={(team.data?.members ?? []).filter((m) => m.id.startsWith("local:") && m.state === "active").map((m) => ({ id: m.id.slice(6), label: `${m.name} (${m.email})` }))} />}
        <p className="text-xs text-fg-muted">{text("organization.separation")}</p>
        {(save.error || plans.error || ((mode === "other" || !mine) && team.error)) && <p role="alert" className="text-sm text-danger">{save.error?.message ?? plans.error?.message ?? team.error?.message}</p>}
        <Button type="submit" variant="primary" tooltip={text("organization.create")} isDisabled={save.isPending || !selectedPlan || !name.trim() || ((mode === "other" || !mine) && !owner)}>{text("organization.create")}</Button>
      </Form>
    </ModalDialog>
  </>;
}
