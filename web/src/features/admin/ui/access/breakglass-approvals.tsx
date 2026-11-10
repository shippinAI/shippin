import { Check, Siren, X } from "lucide-react";
import { Button } from "../../../../lib/design/components/button/button";
import { timeAgo } from "../../../../lib/format";
import { useBreakGlassPending, useDecideBreakGlass } from "../../../../lib/hooks/use-admin-access";

/** Approver view: break-glass waiting for someone other than the requester. */
export function BreakGlassApprovals({ enabled }: { enabled: boolean }) {
  const q = useBreakGlassPending(enabled);
  const decide = useDecideBreakGlass();
  const items = q.data?.items ?? [];
  if (!enabled || !items.length) return null;
  return (
    <div className="mb-6 overflow-hidden rounded-xl border border-danger/35 bg-danger/6">
      <div className="flex items-center gap-2 border-b border-danger/25 px-5 py-3 text-sm font-medium text-danger">
        <Siren className="size-4" /> Break-glass waiting for approval · lapses after {q.data?.approval_minutes} min
      </div>
      {items.map((g) => {
        const own = g.requester === q.data?.self;
        return (
          <div key={g.id} className="flex items-center gap-4 border-b border-line px-5 py-3 last:border-b-0">
            <div className="min-w-0 flex-1 text-sm">
              <div className="text-fg"><span className="font-medium">{g.requester_name}</span> → {g.tenant_name}{g.workspace_name ? ` / ${g.workspace_name}` : ""} <span className="text-fg-subtle">({g.category})</span></div>
              <div className="text-fg-subtle">{g.reason} · {timeAgo(g.created_at)}</div>
            </div>
            {own ? <span className="text-xs text-fg-subtle">Your request — someone else must decide</span> : (
              <>
                <Button variant="ghost" size="sm" tooltip="Refuse; the requester is told" onPress={() => decide.mutate({ id: g.id, decision: "deny" })}><X className="size-4" /> Deny</Button>
                <Button variant="danger" size="sm" tooltip="Needs a fresh sign-in; starts the access now" onPress={() => decide.mutate({ id: g.id, decision: "approve" })}><Check className="size-4" /> Approve</Button>
              </>
            )}
          </div>
        );
      })}
    </div>
  );
}
