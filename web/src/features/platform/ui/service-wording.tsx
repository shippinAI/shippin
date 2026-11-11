import { Badge } from "../../../lib/design/components/badge/badge";
import type { BuiltInService } from "../api";

/** What each built-in service means to its owner, in the words they tick. */
export const understandings: Record<string, string> = {
  postgres: "I understand PostgreSQL holds every account, setting and host in this panel, and losing it loses them",
  clickhouse: "I understand ClickHouse holds my metrics and event history",
  dragonfly: "I understand Dragonfly holds sign-in rate limits, and sign-in is restricted while it is down",
};

export function serviceBadge(s: BuiltInService) {
  if (s.state === "up") return <Badge tone="success">Running</Badge>;
  if (s.state === "disabled") return <Badge>Not in use</Badge>;
  return <Badge tone="danger">Down</Badge>;
}
