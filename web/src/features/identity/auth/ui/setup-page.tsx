import { useState, type InputHTMLAttributes } from "react";
import { Button } from "../../../../lib/design/components/button/button";
import { toast } from "../../../../lib/design/components/toast/toast-queue";
import { usePublicSettings } from "../../../../lib/hooks/use-api";
import { BrandMark } from "../../../../template/brand-mark";
import { RecoveryCodes } from "./recovery-codes";

const field = "rounded-lg border border-line-control bg-canvas px-3 py-2 text-base";

function SetupField({ placeholder, ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <label className="flex min-w-0 flex-col gap-1.5 text-base">
      <span className="font-medium">{placeholder}</span>
      <input {...props} className={field} />
    </label>
  );
}

type Claimed = { username: string; recovery_codes?: string[] };

/** First run (REL-03). There is no setup code and no email address: while the installation has no owner,
 *  the first person to open it chooses a password for the built-in administrator, "admin" unless they
 *  rename it, and is signed in. An installation can also name its owner up front (docs/deploy.md), and
 *  then nobody sees this page. */
export function SetupPage() {
  const s = usePublicSettings().data;
  const [busy, setBusy] = useState(false);
  const [claimed, setClaimed] = useState<Claimed>();
  return (
    <div className="flex min-h-full items-center justify-center overflow-auto p-6 sm:p-10">
      <div className="w-full max-w-xl py-8">
        <div className="mb-8 flex items-center gap-3">
          <BrandMark size={36} logoURL={s?.brand.logo_url} />
          <span className="text-xl font-semibold tracking-tight">{s?.brand.name}</span>
        </div>
        {claimed ? (
          <div data-testid="setup-done">
            {claimed.recovery_codes && claimed.recovery_codes.length > 0 ? (
              <RecoveryCodes
                codes={claimed.recovery_codes}
                next={
                  <Button variant="primary" tooltip="Open the panel" onPress={() => window.location.assign("/admin/start")}>
                    I saved them, continue
                  </Button>
                }
              />
            ) : (
              <Button variant="primary" tooltip="Open the panel" onPress={() => window.location.assign("/admin/start")}>
                Continue
              </Button>
            )}
          </div>
        ) : s && !s.setup_open ? (
          <div>
            <h1 className="text-3xl font-semibold">This installation has an owner</h1>
            <p className="mt-2 text-base text-fg-muted">Sign in to continue.</p>
            <Button className="mt-6" variant="primary" tooltip="Sign in" onPress={() => window.location.assign("/login")}>
              Go to sign in
            </Button>
          </div>
        ) : (
          <>
            <h1 className="text-3xl font-semibold tracking-tight">Set the administrator password</h1>
            <p className="mt-2 text-base text-fg-muted">
              This installation is new. Choose a password for its administrator account, which can do everything in it.
            </p>
            <form
              className="mt-6 grid gap-3 rounded-xl border border-line bg-surface p-5"
              data-testid="setup-form"
              onSubmit={(e) => {
                e.preventDefault();
                const f = new FormData(e.currentTarget);
                const password = String(f.get("password") ?? "");
                if (password !== String(f.get("password_confirm") ?? "")) {
                  toast.error("Setup failed", "Passwords do not match.");
                  return;
                }
                setBusy(true);
                void fetch("/v1/setup", {
                  method: "POST",
                  headers: { "content-type": "application/json" },
                  body: JSON.stringify({ username: f.get("username"), password }),
                })
                  .then(async (res) => {
                    const body = (await res.json().catch(() => ({}))) as Claimed & { message?: string };
                    if (!res.ok) {
                      toast.error("Setup failed", body.message ?? "Check the fields, then try again.");
                      return;
                    }
                    setClaimed(body);
                  })
                  .catch(() => toast.error("Setup failed", "The panel could not be reached. Try again."))
                  .finally(() => setBusy(false));
              }}
            >
              <SetupField
                name="username"
                required
                minLength={3}
                maxLength={32}
                defaultValue="admin"
                autoComplete="username"
                autoCapitalize="none"
                spellCheck={false}
                placeholder="Username"
              />
              <div className="grid gap-3 sm:grid-cols-2">
                <SetupField
                  name="password"
                  type="password"
                  required
                  minLength={12}
                  maxLength={1024}
                  autoComplete="new-password"
                  placeholder="Password (12+ characters)"
                />
                <SetupField
                  name="password_confirm"
                  type="password"
                  required
                  minLength={12}
                  autoComplete="new-password"
                  placeholder="Confirm password"
                />
              </div>
              <Button
                type="submit"
                variant="primary"
                size="lg"
                className="mt-2 w-full"
                isDisabled={busy}
                tooltip="Creates the administrator account and signs you in"
              >
                Continue
              </Button>
            </form>
            <p className="mt-3 text-sm text-fg-subtle">
              Your name, email address, organization, branding and sign-in providers are set in the panel afterwards.{" "}
              <a className="underline" href="/login">
                Sign in instead
              </a>
            </p>
          </>
        )}
      </div>
    </div>
  );
}
