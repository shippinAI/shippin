import { Fingerprint, X } from "lucide-react";
import { useState } from "react";
import type { CredRef, GoldFacts } from "../../../lib/api/gold-sign-types";
import { Button } from "../../../lib/design/components/button/button";
import { toast } from "../../../lib/design/components/toast/toast-queue";
import { signDecision } from "../../../lib/gold-sign/webauthn";
import { useRelay } from "../use-gold-sign";
import { FirstPasskey } from "./first-passkey";

type Props = { t: GoldFacts; rpId: string; keys: CredRef[]; commandsSeen: boolean };

/** Approve and Deny, equal size, side by side. Both need a passkey tap; both are bound to the sha256. */
export function DecisionBar({ t, rpId, keys, commandsSeen }: Props) {
  const relay = useRelay();
  const [busy, setBusy] = useState<"" | "approve" | "deny">("");
  const decide = async (decision: "approve" | "deny") => {
    setBusy(decision);
    try {
      const env = await signDecision({ type: "approval", ticket_id: t.id, ticket_sha256: t.sha256, decision, ts: Math.floor(Date.now() / 1000) }, rpId, keys);
      await relay.mutateAsync(env);
      toast.success(decision === "approve" ? "Approved" : "Denied", "Sent to Gold. Gold verifies your passkey before anything is written.");
    } catch (e) {
      toast.error("Not sent", e instanceof Error ? e.message : "Passkey cancelled");
    } finally {
      setBusy("");
    }
  };
  const noKey = keys.length === 0;
  if (noKey) return <div className="sticky bottom-0 z-20 -mx-4 bg-canvas/90 px-4 py-3 sm:mx-0"><FirstPasskey /></div>;
  return (
    <div className="sticky bottom-0 z-20 -mx-4 border-t border-line bg-canvas/90 px-4 py-3 backdrop-blur sm:mx-0 sm:rounded-xl sm:border">
      {!commandsSeen && <p className="mb-2 text-xs text-fg-muted" data-testid="scroll-hint">Scroll to the end of the commands to enable Approve. Deny is always available.</p>}
      {t.sent && <p className="mb-2 text-xs text-fg-muted">Already sent to Gold: {t.sent}. A new decision replaces it once Gold verifies it.</p>}
      <div className="grid grid-cols-2 gap-3">
        <Button variant="danger" size="lg" className="h-14 w-full justify-center text-base" isDisabled={busy !== ""}
          tooltip="Deny with your passkey" onPress={() => decide("deny")}>
          <X className="size-5" aria-hidden /> {busy === "deny" ? "Waiting…" : "Deny"}
        </Button>
        <Button variant="primary" size="lg" className="h-14 w-full justify-center text-base" isDisabled={!commandsSeen || busy !== ""}
          tooltip={`Approve sha256 ${t.sha256.slice(0, 12)}… with your passkey`} onPress={() => decide("approve")}>
          <Fingerprint className="size-5" aria-hidden /> {busy === "approve" ? "Waiting…" : "Approve"}
        </Button>
      </div>
    </div>
  );
}
