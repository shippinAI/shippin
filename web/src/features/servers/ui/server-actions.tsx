import { Ban, KeyRound } from "lucide-react";
import { useState } from "react";
import { IconButton } from "../../../lib/design/components/button/icon-button";
import { ConfirmDialog } from "../../../lib/design/components/dialog/confirm-dialog";
import { ModalDialog } from "../../../lib/design/components/dialog/modal-dialog";
import type { EnrollToken, Server } from "../../../lib/api/server-types";
import { useIssueToken, useRevokeServer } from "../../../lib/hooks/use-servers";
import { TokenReveal } from "./token-reveal";

/** Row actions: a fresh one-time token (voids the previous one) and revoke. Planned sites have none. */
export function ServerActions({ tenantId, server }: { tenantId: string; server: Server }) {
  const issue = useIssueToken(tenantId);
  const revoke = useRevokeServer(tenantId);
  const [token, setToken] = useState<EnrollToken | null>(null);
  const [confirm, setConfirm] = useState(false);
  if (server.estate || server.state === "planned" || server.state === "revoked") return <span className="text-xs text-fg-subtle">—</span>;
  return (
    <div className="flex justify-end gap-1">
      <IconButton icon={KeyRound} label={`New enrollment token for ${server.label}`} isDisabled={issue.isPending}
        onPress={() => issue.mutate(server.id, { onSuccess: setToken })} />
      <IconButton icon={Ban} label={`Revoke ${server.label}`} onPress={() => setConfirm(true)} />
      <ModalDialog isOpen={!!token} onOpenChange={(o) => !o && setToken(null)} title="Enrollment token"
        description={`For ${server.label}. Any earlier unused token no longer works.`}>
        {token && <TokenReveal token={token} onDone={() => setToken(null)} />}
      </ModalDialog>
      <ConfirmDialog isOpen={confirm} onOpenChange={setConfirm} title={`Revoke ${server.label}?`} confirmLabel="Revoke server"
        typeToConfirm={server.slug} onConfirm={() => revoke.mutate(server.id)}
        body={<p>hostd on this machine loses its credential at its next heartbeat. Work placed on it is not moved. This can't be undone.</p>} />
    </div>
  );
}
