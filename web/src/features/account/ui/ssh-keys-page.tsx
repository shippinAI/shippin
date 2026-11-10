import { KeyRound, Trash2 } from "lucide-react";
import { useState } from "react";
import { IconButton } from "../../../lib/design/components/button/icon-button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { ConfirmDialog } from "../../../lib/design/components/dialog/confirm-dialog";
import { DataTable, type ColumnDef } from "../../../lib/design/components/data-table/data-table";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import type { SSHKey } from "../../../lib/api/types";
import { timeAgo } from "../../../lib/format";
import { useDeleteSSHKey, useSSHKeys } from "../../../lib/hooks/use-ssh-keys";
import { PageHeader } from "../../../template/page-header";
import { AddKeyForm } from "./add-key-form";

export function SSHKeysPage() {
  const q = useSSHKeys();
  const del = useDeleteSSHKey();
  const [target, setTarget] = useState<SSHKey | null>(null);
  const cols: ColumnDef<SSHKey>[] = [
    { id: "name", label: "Name", isRowHeader: true, render: (k) => <span className="flex items-center gap-2 font-medium text-fg"><KeyRound className="size-4 text-hue-secrets" />{k.name}</span> },
    { id: "fp", label: "Fingerprint", render: (k) => <span className="font-mono text-sm">{k.fingerprint}</span> },
    { id: "added", label: "Added", render: (k) => timeAgo(k.created_at) },
    { id: "x", label: "", width: "60px", render: (k) => <IconButton icon={Trash2} label={`Delete key ${k.name}`} onPress={() => setTarget(k)} /> },
  ];
  const items = q.data?.items ?? [];
  return (
    <>
      <PageHeader eyebrow="Account" title="SSH keys" description="Keys you add here are installed on every workspace you own, new and running. They're optional: the browser IDE needs no keys." />
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[1.5fr_1fr]">
        <Card elevation={2} className="overflow-hidden">
          <CardHeader title={`${items.length} key${items.length === 1 ? "" : "s"}`} />
          <DataTable label="SSH keys" columns={cols} rows={items} empty={<EmptyState title="No SSH keys" description="Paste a public key or upload your id_ed25519.pub to use SSH or desktop VS Code." />} />
        </Card>
        <AddKeyForm />
      </div>
      <ConfirmDialog
        isOpen={!!target}
        onOpenChange={(o) => !o && setTarget(null)}
        title={`Delete key "${target?.name}"?`}
        body={<p>The key is removed from all your workspaces. Sessions using it will no longer be able to reconnect.</p>}
        confirmLabel="Delete key"
        onConfirm={() => target && del.mutate(target)}
      />
    </>
  );
}
