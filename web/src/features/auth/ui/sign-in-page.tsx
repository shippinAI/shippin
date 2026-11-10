import { passkeyProblem, signInWithPasskey } from "../../../lib/passkey";
import { ArrowRight, LockKeyhole } from "lucide-react";
import { useEffect } from "react";
import { useSearchParams } from "../../../lib/router/router";
import { Button } from "../../../lib/design/components/button/button";
import { toast } from "../../../lib/design/components/toast/toast-queue";
import { usePublicSettings } from "../../../lib/hooks/use-api";
import { BrandMark } from "../../../template/brand-mark";
import { SignInHero } from "./sign-in-hero";

const errors: Record<string, string> = {
  expired: "The sign-in took too long. Please try again.",
  state: "The sign-in response didn't match. Please try again.",
  exchange: "The identity provider rejected the sign-in.",
};

export function SignInPage() {
  const s = usePublicSettings().data;
  const [params] = useSearchParams();
  const err = params.get("error");
  useEffect(() => {
    if (err) toast.error("Sign-in failed", errors[err] ?? "Unknown error.");
  }, [err]);
  const idpHost = s?.idp_host.replace(/^https?:\/\//, "");
  return (
    <div className="grid h-full grid-cols-1 lg:grid-cols-[1.1fr_1fr]">
      <SignInHero tagline={s?.brand.tagline ?? ""} />
      <div className="flex items-center justify-center p-10">
        <div className="w-full max-w-sm">
          <div className="mb-8 flex items-center gap-3">
            <BrandMark size={36} logoURL={s?.brand.logo_url} />
            <span className="text-xl font-semibold tracking-tight">{s?.brand.name}</span>
          </div>
          <h1 className="text-3xl font-semibold tracking-tight">Welcome back</h1>
          <p className="mt-2 text-base text-fg-muted">
            {s?.app === "operator" ? "Sign in to the provider panel. Staff roles only; every action is audited." : "Sign in to your console. You'll land in your own tenant."}
          </p>
          <div className="mt-8 rounded-xl border border-line bg-surface p-5 shadow-e2">
            {s?.login_ready && (
              <Button
                variant="primary"
                size="lg"
                className="w-full"
                tooltip={`Continue to ${idpHost}`}
                onPress={() => window.location.assign("/auth/login")}
              >
                Continue with {s.idp_name || idpHost} <ArrowRight className="size-4" />
              </Button>
            )}
            {s?.login_ready && (s.login_providers ?? []).map((p) => (
              <Button
                key={p.id}
                variant="secondary"
                size="lg"
                className="mt-3 w-full"
                tooltip={`Sign in with ${p.label} via ${idpHost}`}
                onPress={() => window.location.assign(`/auth/login?idp=${encodeURIComponent(p.id)}`)}
              >
                Continue with {p.label} <ArrowRight className="size-4" />
              </Button>
            ))}
            {(s?.sign_in_providers ?? []).map((p) => (
              <Button
                key={p.id}
                variant={s?.login_ready ? "secondary" : "primary"}
                size="lg"
                className="mb-3 w-full"
                tooltip={`Sign in with ${p.label}`}
                onPress={() => window.location.assign(`/auth/upstream/${encodeURIComponent(p.id)}`)}
              >
                Continue with {p.label} <ArrowRight className="size-4" />
              </Button>
            ))}
            {s?.local_login !== false && (
              <form className="mb-4 grid gap-2" data-testid="password-sign-in" onSubmit={(e) => {
                e.preventDefault(); const f = new FormData(e.currentTarget);
                void fetch("/v1/session/password", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ identifier: f.get("identifier"), password: f.get("password") }) }).then((res) => {
                  if (res.ok) window.location.assign("/"); else toast.error("Sign-in failed", "Username or password is incorrect.");
                }).catch(() => toast.error("Sign-in failed", "The panel could not complete sign-in."));
              }}>
                <input name="identifier" required autoComplete="username" aria-label="Username or email" placeholder="Username or email" className="rounded-lg border border-line-control bg-canvas px-3 py-2 text-base" />
                <input name="password" type="password" required autoComplete="current-password" aria-label="Password" placeholder="Password" className="rounded-lg border border-line-control bg-canvas px-3 py-2 text-base" />
                <Button type="submit" variant="primary" size="lg" className="w-full" tooltip="Sign in with your username or email and password">Sign in with password</Button>
              </form>
            )}
            {s?.local_login !== false && (
              <form
                className="grid gap-2"
                data-testid="passkey-sign-in"
                onSubmit={(e) => {
                  e.preventDefault();
                  const email = String(new FormData(e.currentTarget).get("email") ?? "").trim();
                  void signInWithPasskey(email).then(() => window.location.assign("/")).catch((err: unknown) => toast.error("Sign-in failed", passkeyProblem(err)));
                }}
              >
                <input name="email" type="email" required autoComplete="username webauthn" aria-label="Email" placeholder="you@example.invalid" className="rounded-lg border border-line-control bg-canvas px-3 py-2 text-base text-fg" />
                <Button type="submit" variant="primary" size="lg" className="w-full" tooltip="Use a passkey saved on this device or a security key">Sign in with a passkey</Button>
              </form>
            )}
            {s?.local_login !== false && s?.email_sign_in && (
              <form
                className="mt-4 grid gap-2"
                onSubmit={(e) => {
                  e.preventDefault();
                  const email = new FormData(e.currentTarget).get("email");
                  void fetch("/v1/session/magic", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ email }) }).then((res) => {
                    if (res.ok) toast.success("Check your email", "The sign-in link is on its way.");
                    else toast.error("Sign-in failed", "The panel could not send a link.");
                  });
                }}
              >
                <input name="email" type="email" required placeholder="you@example.invalid" className="rounded-lg border border-line-control bg-canvas px-3 py-2 text-base" />
                <Button type="submit" variant="secondary" size="lg" className="w-full" tooltip="We email a one-time sign-in link. It expires after a few minutes.">Email me a sign-in link</Button>
              </form>
            )}
            {s?.local_login !== false && (
              <details className="mt-4 text-base">
                <summary className="cursor-pointer text-fg-muted">Use a recovery code</summary>
                <form
                  className="mt-2 grid gap-2"
                  onSubmit={(e) => {
                    e.preventDefault();
                    const f = new FormData(e.currentTarget);
                    void fetch("/v1/session/recovery", {
                      method: "POST",
                      headers: { "content-type": "application/json" },
                      body: JSON.stringify({ email: f.get("email"), code: f.get("code") }),
                    }).then((res) => {
                      if (res.ok) window.location.assign("/");
                      else toast.error("Sign-in failed", "That email and recovery code did not match.");
                    });
                  }}
                >
                  <input name="email" type="email" required autoComplete="username" placeholder="you@example.invalid" className="rounded-lg border border-line-control bg-canvas px-3 py-2 text-base" />
                  <input name="code" required autoComplete="one-time-code" placeholder="Recovery code" className="rounded-lg border border-line-control bg-canvas px-3 py-2 font-mono text-base" />
                  <Button type="submit" variant="secondary" size="lg" className="w-full" tooltip="Each recovery code works once.">Sign in with a recovery code</Button>
                </form>
              </details>
            )}
            {s?.login_ready && (
              <div className="mt-4 flex items-center gap-2 text-xs text-fg-subtle">
                <LockKeyhole className="size-3.5" />
                {s.idp_name} · OpenID Connect with PKCE via <span className="font-mono text-fg-muted">{idpHost}</span>
              </div>
            )}
          </div>
          {s?.setup_open && (
            <p className="mt-6 text-base">
              New installation? <a className="font-medium underline" href="/setup">Create the owner account</a> with the setup code the installer printed.
            </p>
          )}
          <p className="mt-8 text-xs text-fg-subtle">
            Need access? Ask your organization owner for an invite, or join the waitlist at /v1/waitlist.
          </p>
        </div>
      </div>
    </div>
  );
}
