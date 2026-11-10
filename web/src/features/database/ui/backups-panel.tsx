import { useState } from "react";
import { Archive } from "lucide-react";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { TextField } from "../../../lib/design/components/field/text-field";
import { timeAgo } from "../../../lib/format";
import { useCreateBackup, useDbBackups, useRestoreBackup } from "../../../lib/hooks/use-database";

export function BackupsPanel({ tenantId }: { tenantId: string }) {
  const q = useDbBackups(tenantId);
  const create = useCreateBackup(tenantId);
  const restore = useRestoreBackup(tenantId);
  const [note, setNote] = useState("");
  const items = q.data?.backups ?? [];
  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-[1.2fr_0.8fr]">
      <Card className="overflow-hidden">
        <CardHeader title="Backups" description={q.data?.message ?? "One-click backups of this database."} />
        {items.length === 0 ? (
          <EmptyState
            title="No backup requests yet"
            illustration={<Archive className="size-10 text-hue-data" aria-hidden />}
            description="A request is recorded here. Copying the data waits on the backup service, which is not connected yet."
          />
        ) : (
          <ul>
            {items.map((b) => (
              <li key={b.id} className="flex items-center justify-between gap-3 border-b border-line px-5 py-3 last:border-b-0">
                <div>
                  <div className="text-base text-fg">{b.note || "Backup request"}</div>
                  <div className="mt-1 text-xs text-fg-subtle">{b.created_at ? timeAgo(b.created_at) : ""}</div>
                </div>
                <div className="flex items-center gap-2">
                  <Badge tone="warning">{b.status === "pending_backend" ? "Not copied yet" : b.status}</Badge>
                  <Button size="sm" variant="secondary" tooltip="Restore is recorded as a request. Postgres is not changed." isDisabled={restore.isPending} onPress={() => restore.mutate(b.id)}>
                    Restore
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </Card>
      <Card>
        <CardHeader title="Request a backup" description="This does not dump Postgres yet. It saves the request for the backup service." />
        <form className="space-y-3 p-5" onSubmit={(e) => { e.preventDefault(); create.mutate(note); }}>
          <TextField label="Note" value={note} onChange={setNote} placeholder="Before the import" description="Optional. Shown in the list." />
          <Button type="submit" variant="primary" tooltip="Record a one-click backup request" isDisabled={create.isPending}>Request backup</Button>
        </form>
      </Card>
    </div>
  );
}
