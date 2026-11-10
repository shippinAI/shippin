import { useState } from "react";
import { Button } from "../../../../lib/design/components/button/button";
import { ConfirmDialog } from "../../../../lib/design/components/dialog/confirm-dialog";
import { useOperatorApps, useOperatorPrepare } from "../../../../lib/hooks/use-apps";

/** Operator status for the VM desktop. Install commands stay on the member's own terminal. */
export function TenantAppsPrep({ id }: { id: string }) {
  const q = useOperatorApps(id);
  const prep = useOperatorPrepare(id);
  const [confirm, setConfirm] = useState(false);
  const rows = q.data?.apps ?? [];
  return (
    <div className="border-t border-line pt-3">
      <p className="text-xs font-medium text-fg">Desktop prerequisites</p>
      <p className="mt-1 text-xs text-fg-subtle">
        Members install Claude, Codex, and ChatGPT themselves and sign in with their own accounts. Signed-in: unknown.
        {q.data?.status_detail ? ` ${q.data.status_detail}` : ""}
      </p>
      {q.isError && <p className="mt-1 text-xs text-danger">Status unavailable.</p>}
      {rows.length > 0 && (
        <ul className="mt-2 space-y-1 text-xs text-fg-muted">
          {rows.map((a) => (
            <li key={a.id}>
              <span className="text-fg">{a.title}</span> — {a.status}
            </li>
          ))}
        </ul>
      )}
      <div className="mt-2">
        <Button size="sm" variant="secondary" isDisabled={prep.isPending} tooltip="Install XFCE and the loopback desktop service. Does not install AI apps." onPress={() => setConfirm(true)}>
          Prepare desktop
        </Button>
      </div>
      <ConfirmDialog
        isOpen={confirm}
        onOpenChange={setConfirm}
        title="Prepare this tenant's VM desktop?"
        body="Installs the desktop session inside the tenant VM only. It does not install Claude, Codex, or ChatGPT, and it does not restart a desktop that is already configured."
        confirmLabel="Prepare desktop"
        onConfirm={() => prep.mutate()}
      />
    </div>
  );
}
