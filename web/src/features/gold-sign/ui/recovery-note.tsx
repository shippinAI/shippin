import { Card } from "../../../lib/design/components/card/card";

/** Recovery, stated plainly (docs/passkey-approvals-design.md "Open decisions"). */
export function RecoveryNote() {
  return (
    <Card className="p-4 text-sm text-fg-muted sm:p-5">
      <h2 className="mb-2 text-sm font-semibold text-fg">Recovery</h2>
      <ul className="list-disc space-y-1.5 pl-5">
        <li>Register two passkeys on day one (phone and desktop, or a hardware key kept offline) and pin both on Gold.</li>
        <li>Lost a device: remove its key from <code className="font-mono">/etc/shippin/signer/owner-keys.json</code> on Gold by a T2 ticket. Until then it is still trusted.</li>
        <li>Lost every passkey: <code className="font-mono">gold-approve</code> as user operator over SSH still works; register new passkeys after a fresh Nomen sign-in and pin them.</li>
        <li>Nomen compromised: an attacker could add a passkey here, but Gold ignores any key that is not pinned there. Compare fingerprints before pinning.</li>
      </ul>
    </Card>
  );
}
