import { ShieldAlert } from "lucide-react";
import { useAdminAccess } from "../../../lib/hooks/use-admin";
import { useIdpName } from "../../../lib/hooks/use-idp-name";
import { usePublicSettings } from "../../../lib/hooks/use-api";

/** Tells staff why their access is limited: MFA missing, or riding on the IAM owner fallback. */
export function StaffBanner() {
  const idp = useIdpName();
  const s = useAdminAccess().data?.staff;
  const account = usePublicSettings().data?.idp_account_url;
  if (!s) return null;
  const mfa = s.mfa_required;
  const legacy = s.mode === "enforce" && s.legacy_fallback;
  if (!mfa && !legacy) return null;
  return (
    <div className="relative z-10 flex items-center gap-2 border-b border-warning/30 bg-warning/10 px-8 py-2 text-sm text-warning">
      <ShieldAlert className="size-4 shrink-0" />
      {mfa && s.local
        ? "Your staff roles are off until you add a passkey or sign in through an identity provider."
        : mfa
        ? `Your staff roles are off until you enroll a second factor (passkey, security key or authenticator app) in your account with ${idp}.`
        : s.local
        ? "You're using the installation owner's fallback access, and every use is audited. Add a passkey so your staff role applies directly."
        : `You're signed in through the owner fallback of ${idp}, and every use is audited. Enroll MFA so your staff role applies directly.`}
      {account && <a href={account} target="_blank" rel="noreferrer" className="ml-auto shrink-0 font-medium underline underline-offset-2">Set up MFA</a>}
    </div>
  );
}
