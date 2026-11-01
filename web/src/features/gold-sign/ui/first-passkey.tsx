import { Fingerprint } from "lucide-react";
import { Button } from "../../../lib/design/components/button/button";
import { defaultDeviceName, useAddPasskey } from "../use-add-passkey";

/** Shown in place of Approve/Deny when no passkey exists yet: one tap sets one up on this device. */
export function FirstPasskey() {
  const { add, busy } = useAddPasskey();
  return (
    <div className="rounded-xl border border-line bg-surface p-3" data-testid="first-passkey">
      <p className="mb-2 text-sm text-fg">Set up a passkey on this device to approve. Your fingerprint, face or device PIN unlocks it.</p>
      <Button variant="primary" size="lg" className="h-14 w-full justify-center text-base" isDisabled={busy}
        tooltip="Creates a passkey for your Nomen account on this device" onPress={() => add(defaultDeviceName())}>
        <Fingerprint className="size-5" aria-hidden /> {busy ? "Waiting…" : "Set up passkey"}
      </Button>
    </div>
  );
}
