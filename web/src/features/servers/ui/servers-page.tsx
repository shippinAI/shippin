import { Plus, ServerIcon } from "lucide-react";
import { useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { DataTable, type ColumnDef } from "../../../lib/design/components/data-table/data-table";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { HelpPopover } from "../../../lib/design/components/popover/help-popover";
import type { Server } from "../../../lib/api/server-types";
import { timeAgo } from "../../../lib/format";
import { useNavigate } from "../../../lib/router/router";
import { useServers } from "../../../lib/hooks/use-servers";
import { useTenant } from "../../../lib/hooks/use-tenant";
import { PageHeader } from "../../../template/page-header";
import { QueryView } from "../../admin/ui/query-view";
import { AddServerDialog } from "./add-server-dialog";
import { ServerActions } from "./server-actions";
import { ServerStateBadge } from "./server-state-badge";

/** Servers (CP-P1): machines this org runs on, their heartbeat state, and planned sites. */
export function ServersPage() {
  const { tenant } = useTenant();
  const nav = useNavigate();
  const tid = tenant?.tenant_id ?? "";
  const q = useServers(tid || undefined);
  const [adding, setAdding] = useState(false);
  const canManage = !!tenant?.can_manage;
  const cols: ColumnDef<Server>[] = [
    { id: "name", label: "Server", isRowHeader: true, render: (s) => (
      <span className="flex items-center gap-2 font-medium text-fg"><ServerIcon className="size-4 text-hue-edge" aria-hidden />{s.label}
        <span className="font-mono text-[12px] text-fg-subtle">{s.slug}</span></span>) },
    { id: "site", label: "Site", render: (s) => <span className="font-mono text-[12.5px]">{s.site}</span> },
    { id: "state", label: "Status", render: (s) => <ServerStateBadge state={s.state} /> },
    { id: "host", label: "Host", render: (s) => (s.hostname ? <span className="font-mono text-[12.5px]">{s.hostname} · {s.agent_version}</span> : "—") },
    { id: "seen", label: "Last heartbeat", render: (s) => (s.last_seen_at ? timeAgo(s.last_seen_at) : "Never") },
    { id: "act", label: "Actions", width: "96px", render: (s) => <ServerActions tenantId={tid} server={s} /> },
  ];
  const beat = q.data ? `Heartbeat every ${q.data.heartbeat_seconds}s; stale after ${q.data.missed_beats} missed.` : "";
  return (
    <>
      <PageHeader eyebrow="Control plane" title="Servers"
        description="Machines your organization runs on. Add one, give hostd its one-time token, and watch it check in."
        actions={<>
          <HelpPopover title="How servers enroll">
            <p className="text-[13px] text-fg-muted">Adding a server gives you a single-use token that expires soon. hostd redeems it once for
              its own credential, which only lives on that machine. Every heartbeat renews the credential. Revoke cuts it off at the next beat.</p>
          </HelpPopover>
          <Button variant="primary" isDisabled={!canManage} onPress={() => setAdding(true)}
            tooltip={canManage ? "Register a new machine" : "Needs the org owner or admin role"}>
            <Plus className="size-4" aria-hidden /> Add server
          </Button>
        </>} />
      <Card elevation={2} className="overflow-hidden">
        <CardHeader title="Servers" description={beat} />
        <QueryView isPending={q.isPending} error={q.error} onRetry={() => void q.refetch()}>
          {() => (
            <DataTable label="Servers" columns={cols} rows={q.data?.items ?? []}
              onRowAction={(id) => nav(`/t/${tid}/servers/${id}`)}
              empty={<EmptyState title="No servers yet" description="Add your first machine, then open it to see its guests." />} />
          )}
        </QueryView>
      </Card>
      {tid && <AddServerDialog tenantId={tid} isOpen={adding} onOpenChange={setAdding} />}
    </>
  );
}
