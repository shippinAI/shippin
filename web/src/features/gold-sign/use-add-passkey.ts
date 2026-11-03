import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "../../lib/design/components/toast/toast-queue";
import { createPasskey } from "../../lib/gold-sign/webauthn";
import { beginRegistration, finishRegistration } from "./use-gold-sign";

/** Registers a passkey on this device (server admits only the owner's fresh Nomen sign-in). */
export function useAddPasskey() {
  const qc = useQueryClient();
  const [busy, setBusy] = useState(false);
  const add = async (label: string) => {
    setBusy(true);
    try {
      const k = await finishRegistration(await createPasskey(await beginRegistration(), label.trim()));
      toast.success("Passkey added", `Fingerprint ${k.fingerprint.slice(0, 16)}… Gold trusts it once it is pinned there.`);
      qc.invalidateQueries({ queryKey: ["gold-sign"] });
      return true;
    } catch (e) {
      toast.error("Not added", e instanceof Error ? e.message : "Cancelled");
      return false;
    } finally {
      setBusy(false);
    }
  };
  return { add, busy };
}

/** A sensible default device name; the user can change it on the Passkeys page. */
export function defaultDeviceName(ua = navigator.userAgent): string {
  if (/iPhone/.test(ua)) return "iPhone";
  if (/iPad/.test(ua)) return "iPad";
  if (/Android/.test(ua)) return "Android phone";
  if (/Mac OS X/.test(ua)) return "Mac";
  if (/Windows/.test(ua)) return "Windows PC";
  return "This device";
}
