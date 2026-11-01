import { Link2, Link2Off } from "lucide-react";
import type { AuditChain } from "../../../../lib/api/access-types";
import { Badge } from "../../../../lib/design/components/badge/badge";

/** The result of the server's fresh walk of the audit hash chain, with the head hash to anchor. */
export function AuditChainBadge({ chain }: { chain?: AuditChain }) {
  if (!chain) return null;
  if (!chain.enabled) return <Badge tone="warning">Hash chain not installed</Badge>;
  if (!chain.ok)
    return (
      <Badge tone="danger">
        <Link2Off className="size-3.5" aria-hidden />
        Chain broken at entry {chain.first_bad}
      </Badge>
    );
  return (
    <span className="flex items-center gap-2">
      <Badge tone="success">
        <Link2 className="size-3.5" aria-hidden />
        Chain verified · {chain.rows} entries
      </Badge>
      {chain.head && (
        <code className="hidden font-mono text-[11px] text-fg-muted md:inline" title={`Head hash ${chain.head}`}>
          {chain.head.slice(0, 12)}…
        </code>
      )}
    </span>
  );
}
