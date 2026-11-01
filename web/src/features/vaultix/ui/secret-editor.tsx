import { Pencil, Plus } from "lucide-react";
import { useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { IconButton } from "../../../lib/design/components/button/icon-button";
import { ModalDialog } from "../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../lib/design/components/field/text-field";
import { toast } from "../../../lib/design/components/toast/toast-queue";
import { useSetSecret } from "../use-vaultix";

const NAME = /^[A-Za-z_][A-Za-z0-9_]{0,127}$/;

/** Add a secret (no editKey) or replace one's value (editKey). The value is write-only here. */
export function SecretEditor({ env, editKey }: { env: string; editKey?: string }) {
  const [open, setOpen] = useState(false);
  const [key, setKey] = useState(editKey ?? "");
  const [value, setValue] = useState("");
  const save = useSetSecret(env);
  const bad = key !== "" && !NAME.test(key);
  const close = () => { setOpen(false); setValue(""); if (!editKey) setKey(""); };
  return (
    <>
      {editKey
        ? <IconButton icon={Pencil} label={`Edit ${editKey}`} onPress={() => setOpen(true)} />
        : <Button variant="primary" tooltip={`Add a secret to ${env}`} isDisabled={!env} onPress={() => setOpen(true)}><Plus className="size-4" aria-hidden /> Add secret</Button>}
      <ModalDialog isOpen={open} onOpenChange={(o) => (o ? setOpen(true) : close())}
        title={editKey ? `New value for ${editKey}` : `Add secret to ${env}`} description="Encrypted at rest in your workspace. Only you and your keyring can read it.">
        <form className="flex flex-col gap-3 px-6 py-5" onSubmit={(e) => {
          e.preventDefault();
          save.mutate({ key, value }, {
            onSuccess: () => { toast.success(editKey ? `${key} updated` : `${key} added`, env); close(); },
            onError: (err) => toast.error("Could not save", err.message),
          });
        }}>
          <TextField label="Name" mono value={key} onChange={(v) => setKey(v.toUpperCase())} isReadOnly={!!editKey} isRequired
            isInvalid={bad} description={bad ? "Letters, digits and _; must not start with a digit." : "e.g. DATABASE_URL"} autoFocus={!editKey} />
          <TextField label="Value" mono multiline value={value} onChange={setValue} isRequired autoFocus={!!editKey} />
          <div className="mt-1 flex justify-end gap-2">
            <Button variant="ghost" tooltip="Close without saving" onPress={close}>Cancel</Button>
            <Button variant="primary" type="submit" tooltip="Save to Vaultix" isDisabled={!key || bad || save.isPending}>Save</Button>
          </div>
        </form>
      </ModalDialog>
    </>
  );
}
