import { useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { toast } from "../../../lib/design/components/toast/toast-queue";
import { usePublicSettings } from "../../../lib/hooks/use-api";
import { BrandMark } from "../../../template/brand-mark";

const field = "rounded-[var(--radius-control)] border border-line bg-bg px-3 py-2 text-sm";

/** First owner: redeem the installer's one-time setup code, then show the recovery codes once. */
export function SetupPage() {
  const s = usePublicSettings().data;
  const [codes, setCodes] = useState<string[] | null>(null);
  const [busy, setBusy] = useState(false);
  return (
    <div className="flex h-full items-center justify-center p-10">
      <div className="w-full max-w-md">
        <div className="mb-8 flex items-center gap-3">
          <BrandMark size={36} />
          <span className="text-xl font-semibold tracking-tight">{s?.brand.name}</span>
        </div>
        {codes ? (
          <div>
            <h1 className="text-[28px] font-semibold tracking-tight">Save your recovery codes</h1>
            <p className="mt-2 text-sm text-fg-muted">
              They are shown only this once. Each one signs you in a single time from the sign-in page; keep them somewhere safe.
            </p>
            <ul className="mt-6 grid grid-cols-2 gap-2 rounded-[var(--radius-card)] border border-line bg-surface p-5 font-mono text-sm">
              {codes.map((c) => (
                <li key={c}>{c}</li>
              ))}
            </ul>
            <div className="mt-4 flex gap-3">
              <Button variant="secondary" tooltip="Copy all recovery codes" onPress={() => void navigator.clipboard?.writeText(codes.join("\n"))}>Copy</Button>
              <Button variant="primary" tooltip="Open the panel" onPress={() => window.location.assign("/")}>I saved them, continue</Button>
            </div>
          </div>
        ) : s && !s.setup_open ? (
          <div>
            <h1 className="text-[28px] font-semibold tracking-tight">Setup is done</h1>
            <p className="mt-2 text-sm text-fg-muted">This installation already has an owner.</p>
            <Button className="mt-6" variant="primary" tooltip="Sign in with a passkey, email link or recovery code" onPress={() => window.location.assign("/login")}>Go to sign in</Button>
          </div>
        ) : (
          <div>
            <h1 className="text-[28px] font-semibold tracking-tight">Create the owner account</h1>
            <p className="mt-2 text-sm text-fg-muted">
              Enter the one-time setup code the installer printed (sudo shippin-console setup issues a new one). It expires after 24 hours.
            </p>
            <form
              className="mt-6 grid gap-2 rounded-[var(--radius-card)] border border-line bg-surface p-5"
              onSubmit={(e) => {
                e.preventDefault();
                const f = new FormData(e.currentTarget);
                setBusy(true);
                void fetch("/v1/setup", {
                  method: "POST",
                  headers: { "content-type": "application/json" },
                  body: JSON.stringify({ code: String(f.get("code") ?? "").trim(), email: f.get("email"), name: f.get("name") }),
                })
                  .then(async (res) => {
                    if (!res.ok) {
                      toast.error("Setup failed", "That setup code is not valid or has expired.");
                      return;
                    }
                    const out = (await res.json()) as { recovery_codes?: string[] };
                    setCodes(out.recovery_codes ?? []);
                  })
                  .finally(() => setBusy(false));
              }}
            >
              <input name="code" required autoComplete="one-time-code" placeholder="Setup code" className={`${field} font-mono`} />
              <input name="name" required autoComplete="name" placeholder="Your name" className={field} />
              <input name="email" type="email" required autoComplete="email" placeholder="you@example.invalid" className={field} />
              <Button type="submit" variant="primary" size="lg" className="mt-2 w-full" isDisabled={busy} tooltip="Creates the owner and signs you in">Create owner</Button>
            </form>
          </div>
        )}
      </div>
    </div>
  );
}
