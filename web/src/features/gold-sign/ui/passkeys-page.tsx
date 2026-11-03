import { Plus } from "lucide-react";
import { useState } from "react";
import { PageHeader } from "../../../template/page-header";
import { Button } from "../../../lib/design/components/button/button";
import { Card } from "../../../lib/design/components/card/card";
import { TextField } from "../../../lib/design/components/field/text-field";
import { useAddPasskey } from "../use-add-passkey";
import { useGoldKeys } from "../use-gold-sign";
import { RecoveryNote } from "./recovery-note";

/** Passkey registration. The server admits only the operator's Nomen account with a fresh sign-in. */
export function PasskeysPage() {
  const keys = useGoldKeys();
  const { add: addKey, busy } = useAddPasskey();
  const [label, setLabel] = useState("");
  const add = async () => {
    if (await addKey(label)) setLabel("");
  };
  return (
    <>
      <PageHeader eyebrow="Gold approvals" title="Passkeys"
        description="Only your Nomen account can add a passkey here. A new passkey cannot sign anything until it is pinned on Gold by a reviewed ticket." />
      {keys.isError && <p className="mb-4 text-sm text-danger">Only the Gold approver's Nomen account can manage passkeys.</p>}
      <Card className="mb-4 p-4 sm:p-5">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
          <div className="flex-1"><TextField label="Device name" placeholder="iPhone, MacBook…" value={label} onChange={setLabel} /></div>
          <Button variant="primary" size="lg" isDisabled={!label.trim() || keys.isError || busy} tooltip="Create a passkey on this device" onPress={add}>
            <Plus className="size-4" aria-hidden /> Add passkey
          </Button>
        </div>
      </Card>
      <ul className="mb-6 space-y-2" aria-label="Registered passkeys">
        {(keys.data ?? []).map((k) => (
          <li key={k.kid} className="rounded-xl border border-line bg-surface px-4 py-3">
            <div className="text-sm font-medium text-fg">{k.label} <span className="font-mono text-xs text-fg-subtle">{k.kid}</span></div>
            <div className="mt-1 font-mono text-[12px] break-all text-fg-muted">sha256 {k.fingerprint}</div>
          </li>
        ))}
      </ul>
      <RecoveryNote />
    </>
  );
}
