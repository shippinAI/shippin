import { useState } from "react";
import type { Contact, Customer, Endpoint } from "../../../../lib/api/catalog-types";
import { Button } from "../../../../lib/design/components/button/button";
import { Drawer } from "../../../../lib/design/components/drawer/drawer";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { Select } from "../../../../lib/design/components/select/select";
import { catalogPaths, useCatalogWrite, usePlans } from "../../../../lib/hooks/use-catalog";
import { CatalogPerm, useCan } from "../../../../lib/hooks/use-can";

type Draft = Pick<Customer, "name" | "plan_id"> & { notes: string; contacts: Contact[]; endpoints: Endpoint[] };

const toLines = <T,>(rows: T[], keys: (keyof T)[]) => rows.map((r) => keys.map((k) => r[k]).join(", ")).join("\n");
const fromLines = <T,>(text: string, keys: (keyof T)[]) =>
  text.split("\n").map((l) => l.split(",").map((x) => x.trim())).filter((p) => p[0])
    .map((p) => Object.fromEntries(keys.map((k, i) => [k, p[i] ?? ""])) as T);

/** Create or edit a customer. Status is not editable here (lifecycle lives under Tenants). */
export function CustomerForm({ initial, onClose }: { initial?: Customer; onClose: () => void }) {
  const plans = usePlans();
  const can = useCan();
  const seeContacts = can(CatalogPerm.contactsRead), seeNotes = can(CatalogPerm.notesRead);
  const save = useCatalogWrite(initial ? "Customer updated" : "Draft customer created");
  const [c, setC] = useState<Draft>({ name: initial?.name ?? "", plan_id: initial?.plan_id ?? plans.data?.items[0]?.id ?? "",
    notes: initial?.notes ?? "", contacts: initial?.contacts ?? [], endpoints: initial?.endpoints ?? [] });
  const [contacts, setContacts] = useState(toLines(c.contacts, ["name", "email", "role"]));
  const [endpoints, setEndpoints] = useState(toLines(c.endpoints, ["kind", "host", "note"]));
  const submit = () => save.mutate({ method: initial ? "PUT" : "POST", path: catalogPaths.customer(initial?.id),
    body: { name: c.name, plan_id: c.plan_id, ...(seeNotes ? { notes: c.notes } : {}),
      ...(seeContacts ? { contacts: fromLines<Contact>(contacts, ["name", "email", "role"]), endpoints: fromLines<Endpoint>(endpoints, ["kind", "host", "note"]) } : {}) } },
    { onSuccess: onClose });
  return (
    <Drawer isOpen onOpenChange={(o) => !o && onClose()} title={initial ? `Edit ${initial.name}` : "New customer"}
      subtitle="New customers start as drafts. Provisioning happens later through a Gold ticket."
      footer={<>
        <div className="flex-1" />
        <Button tooltip="Cancel" variant="ghost" onPress={onClose}>Cancel</Button>
        <Button tooltip="Validate and save" variant="primary" isDisabled={save.isPending || !c.name || !c.plan_id} onPress={submit}>{save.isPending ? "Saving…" : "Save customer"}</Button>
      </>}>
      <div className="grid gap-4 px-6 py-5">
        <TextField label="Name" value={c.name} onChange={(v) => setC({ ...c, name: v })} isRequired />
        <Select label="Plan" value={c.plan_id} onChange={(v) => setC({ ...c, plan_id: v })}
          options={(plans.data?.items ?? []).map((p) => ({ id: p.id, label: p.label }))} />
        {seeContacts && <TextField label="Contacts" description="One per line: name, email, role" multiline value={contacts} onChange={setContacts} />}
        {seeContacts && <TextField label="Endpoints" description="One per line: kind (lan, tailnet, public), host, note" multiline mono value={endpoints} onChange={setEndpoints} />}
        {seeNotes && <TextField label="Notes" multiline value={c.notes} onChange={(v) => setC({ ...c, notes: v })} />}
      </div>
    </Drawer>
  );
}
