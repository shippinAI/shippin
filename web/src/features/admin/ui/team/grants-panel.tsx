import { KeyRound, Trash2 } from "lucide-react";
import { useState } from "react";
import type { PermGrant, Team } from "../../../../lib/api/team-types";
import { Badge } from "../../../../lib/design/components/badge/badge";
import { Button } from "../../../../lib/design/components/button/button";
import { IconButton } from "../../../../lib/design/components/button/icon-button";
import { Card } from "../../../../lib/design/components/card/card";
import { DataTable, type ColumnDef } from "../../../../lib/design/components/data-table/data-table";
import { ModalDialog } from "../../../../lib/design/components/dialog/modal-dialog";
import { EmptyState } from "../../../../lib/design/components/empty-state/empty-state";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { usePermGrants, useRevokePerm } from "../../../../lib/hooks/use-perm-grants";
import { QueryView } from "../query-view";
import { GrantDialog } from "./grant-dialog";

type Row = PermGrant & { id: string };

/** Direct grants: one permission for one principal, beyond their roles (Owner grants and revokes). */
export function GrantsPanel({ team, canWrite }: { team: Team; canWrite: boolean }) {
  const q = usePermGrants();
  const revoke = useRevokePerm();
  const [granting, setGranting] = useState(false);
  const [target, setTarget] = useState<Row>();
  const [reason, setReason] = useState("");
  const who = (id: string) => team.members.find((m) => m.id === id)?.name || id;
  const label = (k: string) => q.data?.grantable.find((p) => p.key === k)?.label ?? k;
  const rows: Row[] = (q.data?.items ?? []).map((g) => ({ ...g, id: `${g.subject}/${g.permission}` }));
  const cols: ColumnDef<Row>[] = [
    { id: "who", label: "Member", isRowHeader: true, render: (g) => <span className="font-medium text-fg">{who(g.subject)}</span> },
    { id: "perm", label: "Permission", render: (g) => <div><Badge tone="info">{g.permission}</Badge><div className="mt-1 text-xs text-fg-muted">{label(g.permission)}</div></div> },
    { id: "reason", label: "Reason", render: (g) => <span className="text-[13px] text-fg-muted">{g.reason}</span> },
    { id: "by", label: "Granted", width: "200px", render: (g) => <span className="text-xs text-fg-subtle">{who(g.granted_by)} · {new Date(g.granted_at).toLocaleString()}</span> },
    { id: "x", label: "", width: "56px", render: (g) => canWrite && <IconButton icon={Trash2} label={`Revoke ${g.permission} from ${who(g.subject)}`} onPress={() => setTarget(g)} /> },
  ];
  return (
    <Card className="overflow-hidden">
      <div className="flex items-center justify-between gap-3 px-5 py-4">
        <p className="text-[13px] text-fg-muted">Catalog and customer permissions are deny by default: Owners hold them; anyone else only through a grant listed here.</p>
        {canWrite && <Button variant="primary" size="sm" tooltip="Grant one permission to one member" isDisabled={!q.data} onPress={() => setGranting(true)}><KeyRound className="size-4" /> Grant</Button>}
      </div>
      <QueryView isPending={q.isPending} error={q.error} onRetry={() => void q.refetch()} rows={3}>
        {() => <DataTable label="Direct grants" columns={cols} rows={rows}
          empty={<EmptyState title="No direct grants" description="Nobody but Owners can see plans, prices or customers." />} />}
      </QueryView>
      {granting && q.data && <GrantDialog team={team} perms={q.data.grantable} onClose={() => setGranting(false)} />}
      {target && (
        <ModalDialog isOpen onOpenChange={(o) => !o && setTarget(undefined)} title={`Revoke ${target.permission}`} description={`${who(target.subject)} loses it on their next request. The revoke is audited.`}>
          <div className="px-6 py-5"><TextField label="Reason" value={reason} onChange={setReason} isRequired /></div>
          <div className="flex gap-2 border-t border-line px-6 py-4">
            <div className="flex-1" />
            <Button tooltip="Cancel" variant="ghost" onPress={() => setTarget(undefined)}>Cancel</Button>
            <Button tooltip="Revoke and audit" variant="danger" isDisabled={revoke.isPending || reason.trim().length < 5}
              onPress={() => revoke.mutate({ subject: target.subject, permission: target.permission, reason: reason.trim() }, { onSuccess: () => { setTarget(undefined); setReason(""); } })}>Revoke</Button>
          </div>
        </ModalDialog>
      )}
    </Card>
  );
}
