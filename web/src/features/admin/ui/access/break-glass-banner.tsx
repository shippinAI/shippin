import { Siren } from "lucide-react";
import { Button } from "../../../../lib/design/components/button/button";
import { useAdminAccess } from "../../../../lib/hooks/use-admin";
import { useAdminGrants, useEndGrant } from "../../../../lib/hooks/use-admin-access";
import { grantScope, untilLabel } from "../../../access/ui/grant-meta";

/** Persistent red banner on every provider page while the admin holds active break-glass. */
export function BreakGlassBanner() {
  const me = useAdminAccess().data?.user_id;
  const q = useAdminGrants();
  const end = useEndGrant();
  const mine = (q.data?.items ?? []).filter((g) => g.kind === "breakglass" && g.status === "approved" && g.requester === me);
  if (!mine.length) return null;
  return (
    <div role="alert" className="sticky top-0 z-30 border-b border-danger/50 bg-[linear-gradient(90deg,oklch(0.45_0.17_25),oklch(0.38_0.14_25))] text-white shadow-[var(--shadow-e2)]">
      {mine.map((g) => (
        <div key={g.id} className="flex items-center gap-3 px-8 py-2.5 text-[13px]">
          <Siren className="size-4 animate-pulse motion-reduce:animate-none" />
          <span className="font-semibold tracking-tight">BREAK-GLASS ACTIVE</span>
          <span className="min-w-0 flex-1 truncate text-white/85">
            {g.tenant_name} · {grantScope(g)} · {untilLabel(g.expires_at)} · every action is audited and the customer is notified
          </span>
          <Button size="sm" variant="secondary" isDisabled={end.isPending} tooltip="End emergency access now" onPress={() => end.mutate(g)}>
            End now
          </Button>
        </div>
      ))}
    </div>
  );
}
