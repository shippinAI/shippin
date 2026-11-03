import { useState } from "react";
import type { A2UIBlock } from "../../../lib/chat/chat-types";
import { Button } from "../../../lib/design/components/button/button";
import { ConfirmDialog } from "../../../lib/design/components/dialog/confirm-dialog";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import { useCopyApp, usePrepareDesktop, useTenantApps } from "../../../lib/hooks/use-apps";
import { A2UIRenderer } from "../../chat/a2ui/a2ui-renderer";

/** One card per app: the official command, a Copy button, and a one-line description. */
export function AppCards({ tenantId }: { tenantId: string }) {
  const q = useTenantApps(tenantId);
  const copy = useCopyApp(tenantId);
  const prep = usePrepareDesktop(tenantId);
  const [confirm, setConfirm] = useState(false);
  const onCopy = (b: A2UIBlock) => {
    if (b.name) copy.mutate(b.name);
  };
  if (q.isPending) return <Skeleton className="h-24" />;
  if (q.isError || !q.data) return <p className="text-sm text-fg-muted">App status is unavailable. The install commands are on the cards once the console can read the VM.</p>;
  return (
    <div>
      {q.data.status_detail && <p className="mb-2 text-xs text-fg-muted">{q.data.status_detail}</p>}
      {q.data.can_prepare && (
        <div className="mb-3">
          <Button size="sm" variant="secondary" isDisabled={prep.isPending} tooltip="Install the desktop session on the VM. This does not install Claude, Codex, or ChatGPT." onPress={() => setConfirm(true)}>
            Prepare desktop
          </Button>
        </div>
      )}
      <A2UIRenderer blocks={q.data.blocks} onPrompt={() => {}} onCopy={onCopy} />
      <ConfirmDialog
        isOpen={confirm}
        onOpenChange={setConfirm}
        title="Prepare the VM desktop?"
        body="This installs XFCE, a desktop service bound to 127.0.0.1:3389, and the packages Claude Cowork needs. It does not install Claude, Codex, or ChatGPT, and it does not restart a desktop that is already configured. You install each app yourself and sign in with your own account."
        confirmLabel="Prepare desktop"
        onConfirm={() => prep.mutate()}
      />
    </div>
  );
}
