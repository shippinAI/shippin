import { useEffect, useState } from "react";
import { Button } from "../../../../lib/design/components/button/button";
import { ModalDialog } from "../../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { Select } from "../../../../lib/design/components/select/select";
import { useCustomers } from "../../../../lib/hooks/use-catalog";
import { useAssignGuest, type Guest } from "../../../../lib/hooks/use-hosts";
import { useText } from "../../../../lib/ui-config/use-ui-config";
import { noteOK } from "../model";

const BY_EMAIL = "";

/** Places a guest under a customer: one that exists, or the customer of an email address, which is
 *  made when that person has none. Nothing on the hypervisor changes. */
export function AssignGuestDialog({ base, guest, onClose }: { base: string; guest: Guest | null; onClose: () => void }) {
  const t = useText();
  const customers = useCustomers();
  const assign = useAssignGuest(base);
  const [customer, setCustomer] = useState(BY_EMAIL);
  const [email, setEmail] = useState("");
  const [note, setNote] = useState("");
  useEffect(() => {
    if (guest) {
      setCustomer(guest.owner?.customer_id ?? BY_EMAIL);
      setEmail("");
      setNote("");
    }
  }, [guest]);
  const options = [
    { id: BY_EMAIL, label: t("infra.assign.byEmail") },
    ...(customers.data?.items ?? []).map((c) => ({ id: c.id, label: c.name })),
  ];
  const emailOK = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim());
  const ready = noteOK(note) && (customer !== BY_EMAIL || emailOK) && !assign.isPending;
  const submit = () => {
    if (!guest || !ready) return;
    assign.mutate(
      { guest: guest.id, note: note.trim(), ...(customer === BY_EMAIL ? { email: email.trim() } : { customerId: customer }) },
      { onSuccess: onClose },
    );
  };
  return (
    <ModalDialog
      isOpen={guest !== null}
      onOpenChange={(o) => {
        if (!o) onClose();
      }}
      title={t("infra.assign.title", { label: guest?.name || guest?.id || "" })}
      description={t("infra.assign.body")}
    >
      <form
        className="space-y-4 px-6 py-5"
        data-testid="assign-guest"
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        <Select label={t("infra.assign.customer")} options={options} value={customer} onChange={setCustomer} />
        {customer === BY_EMAIL && (
          <TextField
            label={t("infra.assign.email")}
            description={t("infra.assign.emailHelp")}
            type="email"
            value={email}
            onChange={setEmail}
            autoFocus
          />
        )}
        <TextField label={t("infra.note.label")} description={t("infra.note.help")} value={note} onChange={setNote} />
        <div className="flex justify-end gap-2 border-t border-line pt-4">
          <Button variant="ghost" tooltip={t("infra.dialog.cancelTooltip")} onPress={onClose}>
            {t("infra.dialog.cancel")}
          </Button>
          <Button type="submit" variant="primary" isDisabled={!ready} tooltip={t("infra.assign.tooltip")}>
            {t("infra.assign.confirm")}
          </Button>
        </div>
      </form>
    </ModalDialog>
  );
}
