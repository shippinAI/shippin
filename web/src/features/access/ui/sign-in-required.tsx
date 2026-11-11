import { KeyRound } from "lucide-react";
import { Button } from "../../../lib/design/components/button/button";
import { Card } from "../../../lib/design/components/card/card";
import type { TenantRef } from "../../../lib/api/types";
import { useText } from "../../../lib/ui-config/use-ui-config";

/** Shown in place of an organization's pages when its plan needs a stronger sign-in than this session used. */
export function SignInRequired({ tenant }: { tenant: TenantRef }) {
  const text = useText();
  const body = tenant.sign_in === "identity_provider" ? text("signInRequired.identityProvider") : text("signInRequired.secondFactor");
  return (
    <div role="alert" className="mx-auto mt-10 max-w-xl">
      <Card elevation={2} className="flex flex-col gap-4 p-8">
        <div className="flex items-center gap-3">
          <KeyRound className="size-5 text-warning" />
          <h1 className="text-lg font-semibold">{text("signInRequired.title", { name: tenant.name })}</h1>
        </div>
        <p className="text-base text-fg-muted">{body}</p>
        <div>
          <Button variant="primary" tooltip={text("signInRequired.signOut")} onPress={() => window.location.assign("/auth/logout")}>{text("signInRequired.signOut")}</Button>
        </div>
      </Card>
    </div>
  );
}
