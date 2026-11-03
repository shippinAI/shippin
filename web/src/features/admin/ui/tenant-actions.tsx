import { useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { ConfirmDialog } from "../../../lib/design/components/dialog/confirm-dialog";
import type { AdminTenant } from "../../../lib/api/admin-types";
import { useTenantAction } from "../../../lib/hooks/use-admin-actions";

/** Retry provisioning, suspend/resume, and typed-name delete for one tenant. */
export function TenantActions({ tenant: t, onDeleted }: { tenant: AdminTenant; onDeleted: () => void }) {
  const act = useTenantAction();
  const [confirm, setConfirm] = useState<"suspend" | "delete">();
  const busy = act.isPending;
  const run = (action: "provision" | "suspend" | "resume" | "delete", typed?: string) =>
    act.mutate({ id: t.id, name: t.name, action, confirm: typed }, { onSuccess: () => action === "delete" && onDeleted() });
  const dead = t.status === "deleted";
  return (
    <>
      <Button size="sm" variant="secondary" isDisabled={busy || dead || t.status === "suspended"} tooltip="Re-run idempotent provisioning (network, project, profile)" onPress={() => run("provision")}>
        Retry provision
      </Button>
      {t.status === "suspended" ? (
        <Button size="sm" variant="secondary" isDisabled={busy} tooltip="Allow workspaces to start again" onPress={() => run("resume")}>Resume</Button>
      ) : (
        <Button size="sm" variant="secondary" isDisabled={busy || t.status !== "active"} tooltip="Stop all workspaces and block starts" onPress={() => setConfirm("suspend")}>Suspend</Button>
      )}
      <div className="flex-1" />
      <Button size="sm" variant="danger" isDisabled={busy || dead} tooltip="Tear down workspaces, network, project and pool" onPress={() => setConfirm("delete")}>
        Delete tenant
      </Button>
      <ConfirmDialog
        isOpen={confirm === "suspend"}
        onOpenChange={(o) => !o && setConfirm(undefined)}
        title={`Suspend ${t.name}?`}
        body={`All ${t.workspace_count} workspace(s) will be stopped and can't be started until you resume the tenant. Data is kept.`}
        confirmLabel="Suspend tenant"
        onConfirm={() => run("suspend")}
      />
      <ConfirmDialog
        isOpen={confirm === "delete"}
        onOpenChange={(o) => !o && setConfirm(undefined)}
        title={`Delete ${t.name}?`}
        body="Every workspace and its home volume, the tenant network (with peers and forwards), the Incus project and storage pool are removed. The encrypted ZFS dataset is kept for recovery. This can't be undone."
        confirmLabel="Delete tenant"
        typeToConfirm={t.name}
        onConfirm={() => run("delete", t.name)}
      />
    </>
  );
}
