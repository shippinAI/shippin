import { ExternalLink } from "lucide-react";
import { Form } from "react-aria-components";
import { useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { ModalDialog } from "../../../lib/design/components/dialog/modal-dialog";
import { CopyField } from "../../../lib/design/components/field/copy-field";
import { TextField } from "../../../lib/design/components/field/text-field";
import type { Connection } from "../../../lib/api/connect-types";
import { useLoginState, useSubmitCode } from "../../../lib/hooks/use-connections";

type Props = { tenantId: string; workspaceId: string; provider: Connection | null; onClose: () => void };

const ended: Record<string, string> = {
  connected: "Connected. The workspace keeps this sign-in across restarts.",
  failed: "The sign-in didn't finish. Close this and try again.",
  expired: "The code expired. Close this and start again.",
};

/** Sign in with browser: the CLI runs inside the workspace; we only show its URL and code. */
export function BrowserLoginDialog({ tenantId, workspaceId, provider, onClose }: Props) {
  const q = useLoginState(tenantId, workspaceId, provider?.id ?? null);
  const submit = useSubmitCode(tenantId, workspaceId);
  const [code, setCode] = useState("");
  const st = q.data;
  return (
    <ModalDialog isOpen={!!provider} onOpenChange={(o) => !o && onClose()} title={`Sign in to ${provider?.label ?? ""}`}
      description={provider?.browser_note}>
      <div className="space-y-4 px-6 py-5" aria-live="polite">
        {!st || st.state === "starting" ? <p className="text-sm text-fg-muted">Starting the sign-in inside your workspace…</p> : null}
        {st?.state === "waiting" && (
          <>
            {st.user_code && <div className="rounded-xl border border-line"><CopyField label="Your code" value={st.user_code} mono /></div>}
            {st.verification_url && (
              <Button variant="primary" tooltip="Opens the vendor's page in a new tab; enter the code there" onPress={() => window.open(st.verification_url, "_blank", "noopener")}>
                <ExternalLink className="size-4" /> Open {new URL(st.verification_url).host}
              </Button>
            )}
            {st.code_input && (
              <Form className="flex items-end gap-2" onSubmit={(e) => { e.preventDefault(); provider && submit.mutate({ provider: provider.id, code }); setCode(""); }}>
                <div className="flex-1"><TextField label="Code from the sign-in page" value={code} onChange={setCode} type="password" mono /></div>
                <Button type="submit" variant="secondary" tooltip="Types the code into the waiting sign-in" isDisabled={!code || submit.isPending}>Submit</Button>
              </Form>
            )}
          </>
        )}
        {st && ended[st.state] && <p className="text-sm text-fg">{ended[st.state]}</p>}
        <div className="flex justify-end"><Button variant="ghost" tooltip="The sign-in keeps waiting until it times out" onPress={onClose}>Close</Button></div>
      </div>
    </ModalDialog>
  );
}
