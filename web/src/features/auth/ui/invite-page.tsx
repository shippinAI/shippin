import { useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { usePublicSettings } from "../../../lib/hooks/use-api";
import { useSearchParams } from "../../../lib/router/router";
import { BrandMark } from "../../../template/brand-mark";
import { RecoveryCodes } from "./recovery-codes";

const field = "h-10 rounded-[var(--radius-control)] border border-line bg-canvas px-3 text-sm";

/** Someone claims the account made for them, with a one-time link from whoever invited them. */
export function InvitePage() {
  const s = usePublicSettings().data;
  const [params] = useSearchParams();
  const code = params.get("code") ?? "";
  const email = params.get("email") ?? "";
  const [codes, setCodes] = useState<string[] | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  return (
    <div className="flex h-full items-center justify-center p-10">
      <div className="w-full max-w-md">
        <div className="mb-8 flex items-center gap-3">
          <BrandMark size={36} />
          <span className="text-xl font-semibold tracking-tight">{s?.brand.name}</span>
        </div>
        {codes ? <RecoveryCodes codes={codes} /> : (
          <div>
            <h1 className="text-[28px] font-semibold tracking-tight">Join this panel</h1>
            <p className="mt-2 text-sm text-fg-muted">You were invited as <span className="font-mono text-fg">{email}</span>. The link works once.</p>
            <form
              className="mt-6 grid gap-2 rounded-[var(--radius-card)] border border-line bg-surface p-5"
              onSubmit={(e) => {
                e.preventDefault();
                const name = String(new FormData(e.currentTarget).get("name") ?? "").trim();
                setBusy(true);
                setError("");
                void fetch("/v1/invites/redeem", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ code, email, name }) })
                  .then(async (res) => {
                    if (!res.ok) {
                      setError("This link has been used or has expired. Ask for a new one.");
                      return;
                    }
                    setCodes(((await res.json()) as { recovery_codes: string[] }).recovery_codes);
                  })
                  .finally(() => setBusy(false));
              }}
            >
              <label className="text-[13px] text-fg-muted" htmlFor="invite-name">Your name</label>
              <input id="invite-name" name="name" autoComplete="name" className={field} />
              {error && <p role="alert" className="text-[13px] text-danger">{error}</p>}
              <Button type="submit" variant="primary" size="lg" className="mt-2 w-full" isDisabled={busy || !code || !email} tooltip="Claim this account and sign in">Join</Button>
            </form>
          </div>
        )}
      </div>
    </div>
  );
}
