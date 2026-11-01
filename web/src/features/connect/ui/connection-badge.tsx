import { Badge } from "../../../lib/design/components/badge/badge";
import type { Connection } from "../../../lib/api/connect-types";
import { expiresIn } from "../../../lib/expiry";

export function ConnectionBadge({ c }: { c: Connection }) {
  if (c.status === "not_connected") return <Badge tone="neutral">Not connected</Badge>;
  if (c.status === "expiring" && c.expires_at) return <Badge tone="warning">{expiresIn(c.expires_at)}</Badge>;
  return <Badge tone="success">Connected{c.method ? ` · ${c.method === "token" ? "token" : "browser"}` : ""}</Badge>;
}
