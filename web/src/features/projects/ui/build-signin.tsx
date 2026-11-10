import { KeyRound } from "lucide-react";
import { useState } from "react";
import type { AIProvider } from "../../../lib/api/subscription-types";
import { Button } from "../../../lib/design/components/button/button";
import { useActivate, useSubscriptions } from "../../../lib/hooks/use-subscriptions";
import { AITerminalModal } from "../../account/ui/ai-terminal-modal";

/**
 * The build waits for the user's OWN sign-in (M2 AI home). Same guided flow as Account → AI:
 * a private terminal where the vendor's login runs; Shippin only learns "signed in", never a token.
 */
export function BuildSignIn({ tenantId, provider }: { tenantId: string; provider: string }) {
  const subs = useSubscriptions();
  const activate = useActivate(tenantId);
  const [active, setActive] = useState<AIProvider | null>(null);
  const p = subs.data?.providers.find((x) => x.id === provider);
  const home = subs.data?.homes.find((h) => h.tenant_id === tenantId);
  if (!p) return null;
  return (
    <div className="rounded-xl border border-warning/40 bg-warning/8 p-4">
      <p className="text-base font-medium text-fg">Sign in to {p.label} to start building</p>
      <p className="mt-1 text-sm text-fg-muted">
        Your agent runs on your own {p.vendor ?? p.label} subscription. Sign in once; the build starts by itself afterwards.
      </p>
      <Button className="mt-3" size="sm" variant="primary" isDisabled={activate.isPending} tooltip="Opens your private sign-in terminal"
        onPress={() => activate.mutate(p.id, { onSuccess: () => setActive(p) })}>
        <KeyRound className="size-4" /> Sign in to {p.label}
      </Button>
      <AITerminalModal tenantId={tenantId} provider={active} home={home} touchSeconds={subs.data?.touch_seconds ?? 0} onClose={() => setActive(null)} />
    </div>
  );
}
