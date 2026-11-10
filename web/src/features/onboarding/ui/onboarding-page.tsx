import { useText } from "../../../lib/ui-config/use-ui-config";
import { ArrowRight, Boxes, Check, KeyRound, Loader2, LogOut, ShieldCheck } from "lucide-react";
import { useState } from "react";
import { Form } from "react-aria-components";
import { Navigate, useNavigate } from "../../../lib/router/router";
import { Button } from "../../../lib/design/components/button/button";
import { TextField } from "../../../lib/design/components/field/text-field";
import { useMe, usePublicSettings } from "../../../lib/hooks/use-api";
import { useOnboard } from "../../../lib/hooks/use-onboard";
import { BrandMark } from "../../../template/brand-mark";
import { SignInHero } from "../../auth/ui/sign-in-hero";

const steps = [
  { icon: ShieldCheck, text: "A private organization with you as owner" },
  { icon: Boxes, text: "An isolated network and encrypted storage" },
  { icon: KeyRound, text: "A browser IDE for every workspace, over HTTPS" },
];

/** First login with no tenant: name your org, we create it and provision its resources. */
/** Signed in, with nothing to do here yet. */
function NoRoleYet({ needsFactor }: { needsFactor: boolean }) {
  const t = useText();
  return (
    <div className="flex h-full items-center justify-center p-10" data-testid="no-role">
      <div className="max-w-md text-center">
        <h1 className="text-2xl font-semibold tracking-tight">{t(needsFactor ? "welcome.factor.title" : "welcome.noRole.title")}</h1>
        <p className="mt-2 text-base text-fg-muted">{t(needsFactor ? "welcome.factor.body" : "welcome.noRole.body")}</p>
      </div>
    </div>
  );
}

export function OnboardingPage() {
  const me = useMe().data;
  if (me && !me.capabilities.onboarding) return <NoRoleYet needsFactor={me.staff?.mfa_required === true} />;
  return <Onboarding />;
}

function Onboarding() {
  const me = useMe().data;
  const s = usePublicSettings().data;
  const nav = useNavigate();
  const onboard = useOnboard();
  const first = me?.display_name.split(" ")[0] ?? "";
  const [name, setName] = useState(first ? `${first}'s org` : "");
  if (me && me.tenants.length) return <Navigate to={`/t/${me.tenants[0].tenant_id}`} replace />;
  const open = !!me?.capabilities.onboarding;
  const n = name.trim().length;
  const valid = n >= 2 && n <= 48;
  const submit = () => onboard.mutate(name.trim(), { onSuccess: (t) => nav(`/t/${t.tenant_id}/workspaces`, { replace: true }) });
  return (
    <div className="grid h-full grid-cols-1 lg:grid-cols-[1.1fr_1fr]">
      <SignInHero tagline={s?.brand.tagline ?? ""} />
      <div className="flex items-center justify-center p-10">
        <div className="w-full max-w-md">
          <div className="mb-8 flex items-center gap-3">
            <BrandMark size={36} />
            <span className="text-xl font-semibold tracking-tight">{s?.brand.name}</span>
          </div>
          <h1 className="text-3xl font-semibold tracking-tight">{first ? `Welcome, ${first}` : "Welcome"}</h1>
          <p className="mt-2 text-base text-fg-muted">
            {open ? "Create your organization to get started. It takes about a minute." : "Your account isn't part of an organization yet."}
          </p>
          <div className="mt-8 rounded-xl border border-line bg-surface p-6 shadow-e2">
            {open ? (
              <Form onSubmit={(e) => { e.preventDefault(); if (valid) submit(); }} className="space-y-5">
                <TextField label="Organization name" value={name} onChange={setName} autoFocus description="Shown to your teammates. You can change it later." />
                <ul className="space-y-2.5">
                  {steps.map(({ icon: I, text }) => (
                    <li key={text} className="flex items-center gap-2.5 text-sm text-fg-muted">
                      <span className="flex size-6 items-center justify-center rounded-md bg-primary/10 ring-1 ring-primary/20">
                        {onboard.isPending ? <Loader2 className="size-3.5 animate-spin text-primary" /> : onboard.isSuccess ? <Check className="size-3.5 text-success" /> : <I className="size-3.5 text-primary" />}
                      </span>
                      {text}
                    </li>
                  ))}
                </ul>
                <Button type="submit" variant="primary" size="lg" className="w-full" isDisabled={!valid || onboard.isPending} tooltip={valid ? "Create the organization and its resources" : "Use 2 to 48 characters"}>
                  {onboard.isPending ? <><Loader2 className="size-4 animate-spin" /> Setting things up…</> : <>Create organization <ArrowRight className="size-4" /></>}
                </Button>
              </Form>
            ) : (
              <p className="text-base text-fg-muted">Sign-up is invite-only right now. Ask your organization owner to invite {me?.email ?? "you"}.</p>
            )}
          </div>
          <div className="mt-6 flex items-center justify-between text-xs text-fg-subtle">
            <span>{(me?.email || me?.display_name) && `Signed in as ${me?.email || me?.display_name}`}</span>
            <Button variant="ghost" size="sm" tooltip="Sign out of the console" onPress={() => window.location.assign("/auth/logout")}>
              <LogOut className="size-3.5" /> Sign out
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
