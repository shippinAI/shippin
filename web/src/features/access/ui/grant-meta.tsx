import { Siren } from "lucide-react";
import { Badge, type BadgeTone } from "../../../lib/design/components/badge/badge";
import type { Grant, GrantStatus } from "../../../lib/api/access-types";

const tones: Record<GrantStatus, [string, BadgeTone]> = {
  pending: ["Pending", "warning"], approved: ["Active", "success"], denied: ["Denied", "neutral"],
  revoked: ["Revoked", "danger"], expired: ["Expired", "neutral"], ended: ["Ended", "neutral"], cancelled: ["Cancelled", "neutral"],
};

/** Status badge for a grant; break-glass grants carry a red marker. */
export function GrantStatusBadge({ grant: g }: { grant: Grant }) {
  const [label, tone] = tones[g.status] ?? [g.status, "neutral" as BadgeTone];
  return (
    <span className="inline-flex items-center gap-1.5">
      {g.kind === "breakglass" && <Badge tone="danger"><Siren className="size-3" /> Break-glass</Badge>}
      <Badge tone={tone}>{label}</Badge>
    </span>
  );
}

export const grantScope = (g: Grant) => (g.workspace_name ? `Workspace ${g.workspace_name}` : "Whole project space");

export function humanDuration(s: number) {
  if (s % 86400 === 0) return `${s / 86400} day${s === 86400 ? "" : "s"}`;
  if (s % 3600 === 0) return `${s / 3600} hour${s === 3600 ? "" : "s"}`;
  return `${Math.round(s / 60)} min`;
}

/** "ends in 42 min" / "ended 3h ago" for an active or closed grant. */
export function untilLabel(iso?: string) {
  if (!iso) return "";
  const ms = new Date(iso).getTime() - Date.now();
  const m = Math.round(Math.abs(ms) / 60000);
  const span = m < 60 ? `${m} min` : `${Math.floor(m / 60)} h ${m % 60} min`;
  return ms > 0 ? `ends in ${span}` : `ended ${span} ago`;
}

/** "1h" → "1 hour", "30m" → "30 minutes" (config durations). */
export function durationLabel(d: string) {
  const m = /^(\d+)([hm])$/.exec(d);
  if (!m) return d;
  const n = Number(m[1]);
  return `${n} ${m[2] === "h" ? "hour" : "minute"}${n === 1 ? "" : "s"}`;
}
