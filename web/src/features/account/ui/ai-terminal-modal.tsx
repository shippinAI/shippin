import { ExternalLink, RefreshCw, Square } from "lucide-react";
import { Button } from "../../../lib/design/components/button/button";
import { ModalDialog } from "../../../lib/design/components/dialog/modal-dialog";
import { CopyField } from "../../../lib/design/components/field/copy-field";
import type { AIHome, AIProvider } from "../../../lib/api/subscription-types";
import { useCheckSubscription, useCloseTerminal, useTouchTerminal } from "../../../lib/hooks/use-subscriptions";

type Props = {
  tenantId: string;
  provider: AIProvider | null;
  home?: AIHome;
  touchSeconds: number;
  onClose: () => void;
};

/** Guides one sign-in: open my private terminal, run the vendor's login, then check. */
export function AITerminalModal({ tenantId, provider, home, touchSeconds, onClose }: Props) {
  const check = useCheckSubscription(tenantId);
  const stop = useCloseTerminal(tenantId);
  const open = !!provider;
  useTouchTerminal(tenantId, open, touchSeconds);
  const running = home?.utility_state === "running";
  const url = home?.utility_host ? `https://${home.utility_host}/` : "";
  const state = home?.utility_state === "error" ? "Your AI terminal failed to start. Close this and try again." : running ? "Your AI terminal is ready." : home?.utility_state === "stopped" ? "Your AI terminal is stopped. Close this and connect again." : "Starting your AI terminal…";
  return (
    <ModalDialog isOpen={open} onOpenChange={(o) => !o && onClose()} title={`Connect ${provider?.label ?? ""}`}
      description="Sign in with your own subscription on your private runner. Provider credentials stay there. Workspace access through the central Devagotchi hub requires a separate connection.">
      <div className="space-y-4 px-6 py-5">
        <p className="text-sm text-fg-muted" aria-live="polite">{state}</p>
        <ol className="list-decimal space-y-2 pl-5 text-sm text-fg">
          <li>Open your AI terminal (a new tab).</li>
          <li>Run the command below. Keep it running while you sign in.</li>
          <li>Open the CLI's sign-in link in Chrome, Edge, or your usual browser on the computer with your security key. Enter the current device code when asked.</li>
          <li>Complete the provider's security-key or passkey prompt, then wait for the terminal to confirm sign-in. Come back here and check.</li>
        </ol>
        {provider && <div className="rounded-xl border border-line"><CopyField label="Command" value={provider.login_command} mono /></div>}
        {provider?.device_url && <CopyField label="Provider sign-in address" value={provider.device_url} mono />}
        <p className="text-sm text-fg-muted">Using a YubiKey or Daybreak? Keep the key connected to your computer and use the provider's own sign-in page. If an embedded browser stalls, open a fresh sign-in in your regular browser. Your key does not need to be attached to the cloud runner.</p>
        <p className="text-sm text-fg-muted">If the code or browser session expires, cancel the waiting command with Ctrl+C and run it again. Use the new code and link. Keep Advanced Account Security enabled. A successful sign-in does not by itself confirm Daybreak access or remaining quota.</p>
        {check.isError && <p role="alert" className="text-sm text-fg-muted">{check.error.message}</p>}
        <div className="flex flex-wrap items-center gap-2">
          <Button variant="primary" isDisabled={!running || !url} tooltip="Opens your private terminal in a new tab"
            onPress={() => window.open(url, "_blank", "noopener")}>
            <ExternalLink className="size-4" /> Open terminal
          </Button>
          <Button variant="secondary" isDisabled={!running || !provider || check.isPending} tooltip="Reads whether the vendor CLI reports you as signed in"
            onPress={() => provider && check.mutate(provider.id)}>
            <RefreshCw className="size-4" /> I've signed in, check
          </Button>
          <Button variant="ghost" isDisabled={!running || stop.isPending} tooltip="Stops the terminal now. Your sign-ins stay on your AI home."
            onPress={() => stop.mutate(undefined, { onSuccess: onClose })}>
            <Square className="size-4" /> Stop terminal
          </Button>
        </div>
      </div>
    </ModalDialog>
  );
}
