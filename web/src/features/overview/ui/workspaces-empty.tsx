import { ArrowRight } from "lucide-react";
import { LinkButton } from "../../../lib/design/components/button/link-button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { useWorkspaces } from "../../../lib/hooks/use-workspaces";
import { StateBadge } from "../../workspaces/ui/state-badge";

/** Overview card: recent workspaces, or an empty state pointing at Workspaces. */
export function WorkspacesEmpty({ tenantId }: { tenantId?: string }) {
  const items = useWorkspaces(tenantId).data?.items ?? [];
  const href = `/t/${tenantId}/workspaces`;
  const open = (
    <LinkButton href={href} size="sm" tooltip="Open the Workspaces page">
      Workspaces <ArrowRight className="size-4" />
    </LinkButton>
  );
  return (
    <Card>
      <CardHeader title="Workspaces" description="Dev environments in your tenant" actions={items.length ? open : undefined} />
      {items.length === 0 ? (
        <EmptyState title="No workspaces yet" description="Spin up a full dev box with its own IDE, terminal and SSH access." action={open} />
      ) : (
        <ul className="divide-y divide-line">
          {items.slice(0, 5).map((w) => (
            <li key={w.id} className="flex items-center gap-3 px-5 py-3.5">
              <span className="flex-1 font-medium">{w.name}</span>
              <span className="font-mono text-xs text-fg-subtle">{w.connect ? `${w.connect.host}:${w.connect.port}` : ""}</span>
              <StateBadge state={w.state} />
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}
