import { ExternalLink } from "lucide-react";
import { Form } from "react-aria-components";
import { useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { ModalDialog } from "../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../lib/design/components/field/text-field";
import type { Connection } from "../../../lib/api/connect-types";
import { useSaveToken } from "../../../lib/hooks/use-connections";

type Props = { tenantId: string; workspaceId: string; provider: Connection | null; onClose: () => void };

/** Use token/API key: a masked field; the value goes to Vaultix and is never shown again. */
export function TokenDialog({ tenantId, workspaceId, provider, onClose }: Props) {
  const save = useSaveToken(tenantId, workspaceId);
  const [token, setToken] = useState("");
  const close = () => { setToken(""); onClose(); };
  return (
    <ModalDialog isOpen={!!provider} onOpenChange={(o) => !o && close()} title={`${provider?.label ?? ""}: use a token`}
      description="Stored in Vaultix under this tenant, then added to the workspace on every start. Nobody can read it back here.">
      <Form className="space-y-4 px-6 py-5" onSubmit={(e) => {
        e.preventDefault();
        if (provider) save.mutate({ provider: provider.id, token: token.trim() }, { onSuccess: close });
      }}>
        <TextField label={provider?.token_label ?? "Token"} description={provider?.token_hint} value={token} onChange={setToken}
          type="password" autoComplete="off" mono isRequired />
        <div className="flex flex-wrap items-center justify-between gap-2">
          {provider?.token_url ? (
            <Button variant="ghost" size="sm" tooltip="Opens the vendor's token page" onPress={() => window.open(provider.token_url, "_blank", "noopener")}>
              <ExternalLink className="size-4" /> Get a token
            </Button>
          ) : <span />}
          <div className="flex gap-2">
            <Button variant="ghost" tooltip="Nothing is saved" onPress={close}>Cancel</Button>
            <Button type="submit" variant="primary" tooltip="Stores it in Vaultix and adds it to the workspace" isDisabled={!token.trim() || save.isPending}>Save to Vaultix</Button>
          </div>
        </div>
      </Form>
    </ModalDialog>
  );
}
