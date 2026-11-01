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
            <BrandMark size={36} />
            <span className="text-xl font-semibold tracking-tight">{s?.brand.name}</span>
          </div>
          <h1 className="text-[28px] font-semibold tracking-tight">Welcome back</h1>
          <p className="mt-2 text-sm text-fg-muted">
            {s?.app === "operator" ? "Sign in to the provider panel. Staff roles only; every action is audited." : "Sign in to your console. You'll land in your own tenant."}
          </p>
          <div className="mt-8 rounded-[var(--radius-card)] border border-line bg-surface p-5 shadow-[var(--shadow-e2)]">
            <Button
              variant="primary"
              size="lg"
              className="w-full"
              isDisabled={!s?.login_ready}
              tooltip={s?.login_ready ? `Continue to ${idpHost}` : "Login isn't configured yet"}
              onPress={() => window.location.assign("/auth/login")}
            >
              Continue with {s?.idp_name ?? ""} <ArrowRight className="size-4" />
            </Button>
            {(s?.login_providers ?? []).map((p) => (
              <Button
                key={p.id}
                variant="secondary"
                size="lg"
                className="mt-3 w-full"
                isDisabled={!s?.login_ready}
                tooltip={s?.login_ready ? `Sign in with ${p.label} via ${idpHost}` : "Login isn't configured yet"}
                onPress={() => window.location.assign(`/auth/login?idp=${encodeURIComponent(p.id)}`)}
              >
                Continue with {p.label} <ArrowRight className="size-4" />
              </Button>
            ))}
            <div className="mt-4 flex items-center gap-2 text-xs text-fg-subtle">
              <LockKeyhole className="size-3.5" />
              {s?.idp_name} · OpenID Connect with PKCE via <span className="font-mono text-fg-muted">{idpHost}</span>
            </div>
          </div>
          <p className="mt-8 text-xs text-fg-subtle">
            Need access? Ask your organization owner for an invite.
          </p>
        </div>
      </div>
    </div>
  );
}
