import { Check, Hourglass, X } from "lucide-react";
import { Button } from "../../../../lib/design/components/button/button";
import { timeAgo } from "../../../../lib/format";
import type { Team } from "../../../../lib/api/team-types";
import { useDecideRequest } from "../../../../lib/hooks/use-team";

/** Two-person requests waiting for a second Owner. */
export function RequestsCard({ team, canWrite }: { team: Team; canWrite: boolean }) {
  const decide = useDecideRequest();
  if (!team.requests.length) return null;
  return (
    <div className="mb-5 overflow-hidden rounded-xl border border-warning/30 bg-warning/6">
      <div className="flex items-center gap-2 border-b border-warning/20 px-5 py-3 text-sm font-medium text-warning">
        <Hourglass className="size-4" /> {team.requests.length} change{team.requests.length === 1 ? "" : "s"} waiting for a second Owner
      </div>
      {team.requests.map((q) => {
        const mine = q.requested_by === team.self;
        const what = q.kind === "role" ? `edit role ${q.target}` : q.kind === "invite" ? `invite ${q.target}` : `roles for ${q.target_name}`;
        const detail = (q.payload.roles ?? q.payload.permissions ?? []).join(", ");
        return (
          <div key={q.id} className="flex items-center gap-4 border-b border-line px-5 py-3 last:border-b-0">
            <div className="min-w-0 flex-1 text-sm">
              <div className="text-fg"><span className="font-medium">{q.requested_name}</span> wants to {what}{detail && <> → <span className="font-mono text-xs">{detail}</span></>}</div>
              <div className="text-fg-subtle">{q.reason} · {timeAgo(q.created_at)}</div>
            </div>
            {canWrite && mine && <Button variant="ghost" size="sm" tooltip="Withdraw your request" onPress={() => decide.mutate({ id: q.id, decision: "cancel" })}>Cancel</Button>}
            {canWrite && !mine && (
              <>
                <Button variant="ghost" size="sm" tooltip="Refuse the change" onPress={() => decide.mutate({ id: q.id, decision: "reject" })}><X className="size-4" /> Reject</Button>
                <Button variant="primary" size="sm" tooltip="Apply it now (re-checked against the last-Owner guard)" onPress={() => decide.mutate({ id: q.id, decision: "approve" })}><Check className="size-4" /> Approve</Button>
              </>
            )}
          </div>
        );
      })}
    </div>
  );
}
