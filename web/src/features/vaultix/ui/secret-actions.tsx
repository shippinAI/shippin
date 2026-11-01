import { Copy, Eye, EyeOff, Trash2 } from "lucide-react";
import { useState } from "react";
import { IconButton } from "../../../lib/design/components/button/icon-button";
import { ConfirmDialog } from "../../../lib/design/components/dialog/confirm-dialog";
import { toast } from "../../../lib/design/components/toast/toast-queue";
import { revealSecret, useDeleteSecret } from "../use-vaultix";
import { SecretEditor } from "./secret-editor";

/** Masked value with reveal (auto-hides after 20 s), copy, edit and delete. */
export function SecretActions({ env, secretKey }: { env: string; secretKey: string }) {
  const [shown, setShown] = useState<string | null>(null);
  const [confirm, setConfirm] = useState(false);
  const del = useDeleteSecret(env);
  const fetchValue = async () => {
    try {
      return await revealSecret(env, secretKey);
    } catch (e) {
      toast.error("Could not read the value", (e as Error).message);
      return null;
    }
  };
  const reveal = async () => {
    if (shown !== null) return setShown(null);
    const v = await fetchValue();
    if (v === null) return;
    setShown(v);
    window.setTimeout(() => setShown(null), 20_000);
  };
  const copy = async () => {
    const v = shown ?? (await fetchValue());
    if (v === null) return;
    await navigator.clipboard.writeText(v).then(() => toast.success(`${secretKey} copied`), () => toast.error("Clipboard blocked"));
  };
  return (
    <div className="flex items-center gap-1">
      <code className="mr-2 max-w-[18rem] truncate rounded bg-surface px-2 py-0.5 font-mono text-[12px]" aria-live="polite">
        {shown ?? "••••••••••••"}
      </code>
      <IconButton icon={shown === null ? Eye : EyeOff} label={shown === null ? `Reveal ${secretKey}` : `Hide ${secretKey}`} onPress={reveal} />
      <IconButton icon={Copy} label={`Copy ${secretKey}`} onPress={copy} />
      <SecretEditor env={env} editKey={secretKey} />
      <IconButton icon={Trash2} label={`Delete ${secretKey}`} onPress={() => setConfirm(true)} />
      <ConfirmDialog isOpen={confirm} onOpenChange={setConfirm} title={`Delete ${secretKey}?`}
        body={`It is removed from ${env}. Keyrings drop it on their next sync.`} confirmLabel="Delete"
        onConfirm={() => del.mutate(secretKey, { onSuccess: () => toast.success(`${secretKey} deleted`), onError: (e) => toast.error("Delete failed", e.message) })} />
    </div>
  );
}
