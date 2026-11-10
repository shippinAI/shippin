import { ServerIcon } from "lucide-react";
import { Card } from "../../../lib/design/components/card/card";
import { DataTable, type ColumnDef } from "../../../lib/design/components/data-table/data-table";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import type { Server } from "../../../lib/api/server-types";
import { timeAgo } from "../../../lib/format";
import { useOpsServers } from "../../../lib/hooks/use-orgs";
import { PageHeader } from "../../../template/page-header";
import { ServerStateBadge } from "../../servers/ui/server-state-badge";

/** Operations · Servers: the whole fleet, for staff roles. Needs no devspace or tenant membership. */
export function OpsServersPage() {
  const q = useOpsServers();
  const cols: ColumnDef<Server>[] = [
    { id: "name", label: "Server", isRowHeader: true, render: (s) => (
      <span className="flex items-center gap-2 font-medium text-fg"><ServerIcon className="size-4 text-hue-edge" aria-hidden />{s.label}
        <span className="font-mono text-xs text-fg-subtle">{s.slug}</span></span>) },
    { id: "site", label: "Site", render: (s) => <span className="font-mono text-sm">{s.site}</span> },
    { id: "state", label: "Status", render: (s) => <ServerStateBadge state={s.state} /> },
    { id: "seen", label: "Last heartbeat", render: (s) => (s.last_seen_at ? timeAgo(s.last_seen_at) : "Never") },
  ];
  const denied = q.error && /403|staff role/i.test(q.error.message);
  return (
    <>
      <PageHeader eyebrow="Operations" title="Servers"
        description="Every server in the Shippin.cloud fleet. Staff roles see this without a devspace; customer data needs a support grant." />
      {denied ? (
        <div data-testid="ops-denied"><Card className="p-6">
          <EmptyState title="Operations staff only" description="This needs an Operations staff role. Tenant roles such as org owner don't grant it." />
        </Card></div>
      ) : (
        <div data-testid="ops-servers"><Card className="p-0">
          <DataTable label="Fleet servers" columns={cols} rows={q.data?.items ?? []}
            empty={<EmptyState title={q.isLoading ? "Loading…" : "No servers yet"} description="Servers appear here once added in a customer org or seeded as estate." />} />
        </Card></div>
      )}
    </>
  );
}
