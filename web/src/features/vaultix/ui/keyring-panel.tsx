import { KeyRound } from "lucide-react";
import { useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { CopyField } from "../../../lib/design/components/field/copy-field";
import { ModalDialog } from "../../../lib/design/components/dialog/modal-dialog";
import { toast } from "../../../lib/design/components/toast/toast-queue";
import type { KeyringLogin, VaultixAccount } from "../vaultix-types";
import { useKeyringLogin } from "../use-vaultix";

/** Connect the vaultix-keyring CLI: mints a keyring login shown exactly once. */
export function KeyringPanel({ account, env }: { account: VaultixAccount; env: string }) {
  const mint = useKeyringLogin();
  const [login, setLogin] = useState<KeyringLogin | null>(null);
  const setup = login && [
    `printf '%s\\n%s\\n' "$CLIENT_ID" "$CLIENT_SECRET" | vaultix login --universal --api ${login.api_url}`,
    `vaultix init --project ${login.workspace_slug} --env ${env || "dev"} --path /`,
    "vaultix pull && vaultix hook bash >> ~/.bashrc   # pulls before every new shell",
  ].join("\n");
  return (
    <aside aria-label="Keyring" className="rounded-2xl border border-line bg-raised/60 p-4">
      <h2 className="mb-1 flex items-center gap-2 text-sm font-semibold"><KeyRound className="size-4" aria-hidden /> vaultix-keyring</h2>
      <p className="mb-3 text-[13px] text-fg-muted">
        Keeps an encrypted copy on your machine, creates missing secrets on push and pulls updates before each new session.
      </p>
      <Button variant={account.keyring_connected ? "ghost" : "primary"} isDisabled={mint.isPending}
        tooltip="Creates a new keyring login; older ones keep working until you revoke them"
        onPress={() => mint.mutate(undefined, { onSuccess: setLogin, onError: (e) => toast.error("Could not create a keyring login", e.message) })}>
        {account.keyring_connected ? "New keyring login" : "Connect keyring"}
      </Button>
      <ModalDialog isOpen={!!login} onOpenChange={(o) => !o && setLogin(null)} title="Keyring login"
        description="The client secret is shown once. Paste it into your terminal now; Vaultix only keeps a hash.">
        {login && setup && (
          <div className="flex flex-col gap-3 px-6 py-5">
            <CopyField label="API" value={login.api_url} mono />
            <CopyField label="Client ID" value={login.client_id} mono />
            <CopyField label="Client secret" value={login.client_secret} mono />
            <CopyField label="Setup" value={setup} mono hint="Run with CLIENT_ID and CLIENT_SECRET exported from the fields above." />
            <div className="flex justify-end"><Button variant="primary" tooltip="I saved the secret" onPress={() => setLogin(null)}>Done</Button></div>
          </div>
        )}
      </ModalDialog>
    </aside>
  );
}
