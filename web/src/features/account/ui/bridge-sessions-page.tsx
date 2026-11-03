import { Cable, Trash2 } from "lucide-react";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../../../lib/api/client";
import { IconButton } from "../../../lib/design/components/button/icon-button";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { ConfirmDialog } from "../../../lib/design/components/dialog/confirm-dialog";
import { DataTable, type ColumnDef } from "../../../lib/design/components/data-table/data-table";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { timeAgo } from "../../../lib/format";
import { PageHeader } from "../../../template/page-header";

type BridgeSession = { id: string; client_name: string; email: string; created_at: string; expires_at?: string; last_used_at?: string; status: string };

/** Devices signed in with the Shippin bridge; revoking closes their open shells at once. */
export function BridgeSessionsPage() {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["bridge-sessions"], queryFn: () => apiGet<{ items: BridgeSession[] }>("/v1/me/bridge-sessions") });
  const revoke = useMutation({
    mutationFn: (s: BridgeSession) => apiSend<{ relays_closed: number }>("DELETE", `/v1/me/bridge-sessions/${s.id}`),
    onSettled: () => qc.invalidateQueries({ queryKey: ["bridge-sessions"] }),
  });
  const [target, setTarget] = useState<BridgeSession | null>(null);
  const live = (s: BridgeSession) => s.status === "issued" || s.status === "approved";
  const cols: ColumnDef<BridgeSession>[] = [
    { id: "name", label: "Device", isRowHeader: true, render: (s) => <span className="flex items-center gap-2 font-medium text-fg"><Cable className="size-4 text-hue-secrets" />{s.client_name}</span> },
    { id: "status", label: "Status", render: (s) => <Badge tone={live(s) ? "success" : "neutral"}>{live(s) ? "active" : s.status}</Badge> },
    { id: "used", label: "Last used", render: (s) => (s.last_used_at ? timeAgo(s.last_used_at) : "never") },
    { id: "signed", label: "Signed in", render: (s) => timeAgo(s.created_at) },
    { id: "x", label: "", width: "60px", render: (s) => (live(s) ? <IconButton icon={Trash2} label={`Revoke ${s.client_name}`} onPress={() => setTarget(s)} /> : null) },
  ];
  const items = q.data?.items ?? [];
  return (
    <>
      <PageHeader eyebrow="Account" title="Bridge sessions" description="Devices and local AIs signed in with shippin-bridge. Revoking ends the session now and closes its open shells." />
      <Card elevation={2} className="overflow-hidden">
        <CardHeader title={`${items.filter(live).length} active`} />
        <DataTable label="Bridge sessions" columns={cols} rows={items}
          empty={<EmptyState title="No bridge sessions" description="Run shippin-bridge login on your machine to open devspaces from a terminal or a local AI." />} />
      </Card>
      <ConfirmDialog
        isOpen={!!target}
        onOpenChange={(o) => !o && setTarget(null)}
        title={`Revoke "${target?.client_name}"?`}
        body={<p>The device must sign in again. Any shell it has open is closed immediately.</p>}
        confirmLabel="Revoke session"
        onConfirm={() => target && revoke.mutate(target)}
      />
    </>
  );
}
