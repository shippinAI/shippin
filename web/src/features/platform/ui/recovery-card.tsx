import { useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { buttonClass } from "../../../lib/design/components/button/button-styles";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { CodeBlock } from "../../../lib/design/components/code-block/code-block";
import { StrictDialog } from "../../../lib/design/components/dialog/strict-dialog";
import { CopyField } from "../../../lib/design/components/field/copy-field";
import { TextField } from "../../../lib/design/components/field/text-field";
import { toast } from "../../../lib/design/components/toast/toast-queue";
import { formatBytes } from "../../../lib/format";
import { revealRecoveryKey, useBackups, useCreateBackup, useDeleteBackup, type Backup } from "../api";

/** Backups, the key that opens saved secrets, and how to restore from them. */
export function RecoveryCard() {
  const backups = useBackups();
  const create = useCreateBackup();
  const remove = useDeleteBackup();
  const [reason, setReason] = useState("");
  const [deleting, setDeleting] = useState<Backup | null>(null);
  const [revealing, setRevealing] = useState(false);
  const [key, setKey] = useState("");
  const [busy, setBusy] = useState(false);
  const items = backups.data?.items ?? [];
  const reveal = async (why: string) => {
    setBusy(true);
    try {
      setKey((await revealRecoveryKey(why)).key);
      setRevealing(false);
    } catch (e) {
      toast.error("Key not shown", e instanceof Error ? e.message : "");
    } finally {
      setBusy(false);
    }
  };
  return (
    <Card className="overflow-hidden" data-testid="recovery">
      <CardHeader title="Disaster recovery" description="A backup of PostgreSQL holds your accounts, settings and hosts. Saved secrets also need the recovery key. ClickHouse history is not included." />
      <div className="space-y-5 px-5 py-4">
        <form className="flex flex-wrap items-end gap-3" onSubmit={(e) => { e.preventDefault(); create.mutate(reason.trim(), { onSuccess: () => setReason("") }); }}>
          <div className="min-w-64 flex-1"><TextField label="Why are you backing up?" value={reason} onChange={setReason} /></div>
          <Button type="submit" variant="primary" isDisabled={reason.trim().length < 5 || create.isPending} tooltip="Save a backup on this machine now">{create.isPending ? "Backing up…" : "Back up now"}</Button>
        </form>
        {items.length === 0
          ? <p className="text-base text-fg-muted">No backups yet.</p>
          : (
            <ul className="divide-y divide-line rounded-lg border border-line" aria-label="Backups">
              {items.map((b) => (
                <li key={b.name} className="flex flex-wrap items-center gap-3 px-4 py-2.5">
                  <span className="min-w-0 flex-1 truncate font-mono text-sm text-fg">{b.name}</span>
                  <span className="text-sm text-fg-muted">{formatBytes(b.size_bytes)}</span>
                  <span className="text-sm text-fg-muted">{new Date(b.created_at).toLocaleString()}</span>
                  <a href={`/v1/operator/backups/${b.name}`} download={b.name} title="Download this backup to keep it somewhere else" className={buttonClass("secondary", "sm")}>Download</a>
                  <Button size="sm" variant="ghost" tooltip="Delete this backup" onPress={() => setDeleting(b)}>Delete</Button>
                </li>
              ))}
            </ul>
          )}
        <div className="space-y-2 border-t border-line pt-4">
          <h3 className="text-base font-semibold text-fg">Recovery key</h3>
          <p className="text-base text-fg-muted">Saved host credentials are sealed with this key. Keep it apart from your backups.</p>
          {key
            ? <CopyField label="Recovery key" value={key} mono hint="Shown once on this page. Store it somewhere safe." />
            : <Button size="sm" variant="secondary" tooltip="Show the key that opens saved secrets" onPress={() => setRevealing(true)}>Show recovery key</Button>}
        </div>
        <div className="space-y-2 border-t border-line pt-4">
          <h3 className="text-base font-semibold text-fg">Restoring</h3>
          <p className="text-base text-fg-muted">Restoring replaces the whole database, so it runs from the server with the panel stopped. On a new machine, first put the recovery key in <span className="font-mono">.env</span> as <span className="font-mono">SHIPPIN_SESSION_KEY</span>.</p>
          <CodeBlock label="Restore command" code="scripts/restore-backup.sh shippin-backup.dump --i-understand" />
        </div>
      </div>
      <StrictDialog isOpen={deleting !== null} onClose={() => setDeleting(null)} title="Delete this backup?" confirm="Delete backup" busy={remove.isPending}
        warning={`${deleting?.name ?? ""} is removed from this machine. Copies you downloaded are not affected.`}
        understand="I understand this backup cannot be recovered"
        onConfirm={() => { if (deleting) remove.mutate(deleting.name, { onSuccess: () => setDeleting(null) }); }} />
      <StrictDialog isOpen={revealing} onClose={() => setRevealing(false)} title="Show the recovery key?" confirm="Show key" busy={busy}
        warning="Anyone with this key and a backup can read every secret saved in this panel."
        understand="I understand this key unlocks my saved secrets"
        onConfirm={(why) => void reveal(why)} />
    </Card>
  );
}
