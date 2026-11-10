import { useState, type InputHTMLAttributes } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { toast } from "../../../lib/design/components/toast/toast-queue";
import { usePublicSettings } from "../../../lib/hooks/use-api";
import { BrandMark } from "../../../template/brand-mark";
import { RecoveryCodes } from "./recovery-codes";

const field = "rounded-lg border border-line-control bg-canvas px-3 py-2 text-base";

function SetupField({ placeholder, ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return <label className="flex min-w-0 flex-col gap-1.5 text-base"><span className="font-medium">{placeholder}</span><input {...props} /></label>;
}

/** First run collects real credentials and the installation's initial org and brand. */
export function SetupPage() {
  const s = usePublicSettings().data;
  const [codes, setCodes] = useState<string[] | null>(null);
  const [busy, setBusy] = useState(false);
  return <div className="flex min-h-full items-center justify-center overflow-auto p-6 sm:p-10">
    <div className="w-full max-w-xl py-8">
      <div className="mb-8 flex items-center gap-3"><BrandMark size={36} logoURL={s?.brand.logo_url} /><span className="text-xl font-semibold tracking-tight">{s?.brand.name}</span></div>
      {codes ? <RecoveryCodes codes={codes} /> : s && !s.setup_open ? <div><h1 className="text-3xl font-semibold">Setup is done</h1><p className="mt-2 text-base text-fg-muted">This installation already has an owner.</p><Button className="mt-6" variant="primary" tooltip="Sign in" onPress={() => window.location.assign("/login")}>Go to sign in</Button></div> : <>
        <h1 className="text-3xl font-semibold tracking-tight">Set up {s?.brand.name}</h1>
        <p className="mt-2 text-base text-fg-muted">Create the first owner account and the internal organization that owns infrastructure and platform services. Customer organizations are created separately. The one-time setup code authorizes this first owner; choose a unique password that you will use to sign in.</p>
        <form className="mt-6 grid gap-3 rounded-xl border border-line bg-surface p-5" onSubmit={(e) => {
          e.preventDefault(); const f = new FormData(e.currentTarget); const password = String(f.get("password") ?? "");
          if (password !== String(f.get("password_confirm") ?? "")) { toast.error("Setup failed", "Passwords do not match."); return; }
          setBusy(true);
          void fetch("/v1/setup", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({
            code: f.get("code"), username: f.get("username"), name: f.get("name"), email: f.get("email"), password,
            organization: f.get("organization"), brand: { name: f.get("brand_name"), short: f.get("brand_short"), tagline: f.get("tagline"), logo_url: f.get("logo_url"), color: f.get("color"), privacy_url: f.get("privacy_url"), terms_url: f.get("terms_url") },
          }) }).then(async (res) => {
            if (!res.ok) { const detail = await res.json().catch(() => ({})) as { message?: string; detail?: string }; toast.error("Setup failed", detail.message ?? detail.detail ?? "Check the required fields and setup code, then try again."); return; }
            const out = await res.json() as { recovery_codes?: string[] }; setCodes(out.recovery_codes ?? []);
          }).catch(() => toast.error("Setup failed", "The panel could not complete setup. Your setup code remains usable if setup did not commit.")).finally(() => setBusy(false));
        }}>
          <h2 className="text-base font-semibold">Owner account</h2>
          <SetupField name="code" required autoComplete="one-time-code" placeholder="One-time setup code" className={`${field} font-mono`} />
          <div className="grid gap-3 sm:grid-cols-2"><SetupField name="username" required minLength={3} maxLength={32} pattern="[a-zA-Z0-9][a-zA-Z0-9._-]{2,31}" autoComplete="username" placeholder="Username" className={field} /><SetupField name="name" required maxLength={120} autoComplete="name" placeholder="Full name" className={field} /></div>
          <SetupField name="email" type="email" required autoComplete="email" placeholder="Email address" className={field} />
          <div className="grid gap-3 sm:grid-cols-2"><SetupField name="password" type="password" required minLength={12} maxLength={1024} autoComplete="new-password" placeholder="Password (12+ characters)" className={field} /><SetupField name="password_confirm" type="password" required minLength={12} autoComplete="new-password" placeholder="Confirm password" className={field} /></div>
          <h2 className="mt-3 text-base font-semibold">Internal organization and installation brand</h2>
          <SetupField name="organization" required minLength={2} maxLength={120} placeholder="Internal operator organization" className={field} />
          <div className="grid gap-3 sm:grid-cols-2"><SetupField name="brand_name" required maxLength={80} defaultValue={s?.brand.name ?? "Shippin"} placeholder="Product name" className={field} /><SetupField name="brand_short" maxLength={24} defaultValue={s?.brand.short ?? "Shippin"} placeholder="Short name" className={field} /></div>
          <SetupField name="tagline" maxLength={160} defaultValue={s?.brand.tagline ?? ""} placeholder="Tagline" className={field} />
          <div className="grid gap-3 sm:grid-cols-2"><SetupField name="logo_url" type="url" placeholder="Logo URL (HTTPS)" className={field} /><label className="flex items-center gap-3 rounded-lg border border-line-control bg-canvas px-3 py-2 text-base text-fg-muted">Brand color {/* tokens-allow: a colour input needs a #rrggbb starting value; brand colour is installation data */}<input name="color" type="color" defaultValue="#3559e0" className="ml-auto h-7 w-10 bg-transparent" /></label></div>
          <div className="grid gap-3 sm:grid-cols-2"><SetupField name="privacy_url" type="url" placeholder="Privacy policy URL (optional)" className={field} /><SetupField name="terms_url" type="url" placeholder="Terms URL (optional)" className={field} /></div>
          <Button type="submit" variant="primary" size="lg" className="mt-2 w-full" isDisabled={busy} tooltip="Creates the owner, internal organization, and brand settings">Create installation</Button>
        </form>
        <p className="mt-3 text-xs text-fg-subtle">Brand settings are editable later. Public hostname and TLS are configured in the deployment settings.</p>
      </>}
    </div>
  </div>;
}
