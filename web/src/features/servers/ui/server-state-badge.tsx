import { Badge, type BadgeTone } from "../../../lib/design/components/badge/badge";
import { StatusDot, type Health } from "../../../lib/design/components/status-dot/status-dot";
import { Tooltip } from "../../../lib/design/components/tooltip/tooltip";
import type { ServerState } from "../../../lib/api/server-types";
import { Focusable } from "react-aria-components";

const look: Record<ServerState, { tone: BadgeTone; health: Health; label: string; hint: string }> = {
  planned: { tone: "neutral", health: "unknown", label: "Planned", hint: "A future site. Nothing contacts it yet." },
  pending: { tone: "info", health: "unknown", label: "Waiting to enroll", hint: "Give hostd the enrollment token to finish." },
  active: { tone: "success", health: "up", label: "Active", hint: "Heartbeats are arriving on time." },
  stale: { tone: "warning", health: "down", label: "Stale", hint: "Missed heartbeats. Check hostd and the network." },
  revoked: { tone: "danger", health: "down", label: "Revoked", hint: "Its credential is dead. Add the server again to re-enroll." },
};

/** State as a badge with a dot and a tooltip that says what it means. */
export function ServerStateBadge({ state }: { state: ServerState }) {
  const l = look[state];
  return (
    <Tooltip content={l.hint}>
      <Focusable>
        <span tabIndex={0} role="status" aria-label={`${l.label}: ${l.hint}`} className="focus-ring inline-flex rounded-full outline-none">
          <Badge tone={l.tone}><StatusDot health={l.health} />{l.label}</Badge>
        </span>
      </Focusable>
    </Tooltip>
  );
}
