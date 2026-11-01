import { Globe, KeyRound, LogOut, RefreshCw } from "lucide-react";
import { Button } from "../../../lib/design/components/button/button";
import { Card } from "../../../lib/design/components/card/card";
import type { Connection } from "../../../lib/api/connect-types";
import { ConnectionBadge } from "./connection-badge";

type Props = {
  c: Connection;
  running: boolean;
  onBrowser: () => void;
  onToken: () => void;
  onDisconnect: () => void;
};

/** One provider: status, the two ways to connect, and Reconnect / Disconnect once connected. */
export function ProviderCard({ c, running, onBrowser, onToken, onDisconnect }: Props) {
  const connected = c.status !== "not_connected";
  return (
    <Card elevation={2} className="flex flex-col gap-4 p-5" >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="text-sm font-semibold text-fg">{c.label}</h3>
          <p className="text-xs text-fg-subtle">{c.vendor}</p>
        </div>
        <ConnectionBadge c={c} />
      </div>
      <p className="text-[13px] text-fg-muted">{c.browser_note}</p>
      {c.stored_in_vaultix && <p className="text-xs text-fg-subtle">A token is saved in Vaultix and added on every start.</p>}
      <div className="mt-auto flex flex-wrap gap-2">
        {connected ? (
          <>
            <Button size="sm" variant="secondary" isDisabled={!running} onPress={onBrowser} tooltip="Sign in again with your browser">
              <RefreshCw className="size-4" /> Reconnect
            </Button>
            <Button size="sm" variant="ghost" isDisabled={!running} onPress={onDisconnect} tooltip="Signs out in the workspace and removes the Vaultix copy">
              <LogOut className="size-4" /> Disconnect
            </Button>
          </>
        ) : (
          <>
            <Button size="sm" variant="primary" isDisabled={!running} onPress={onBrowser} tooltip="Runs the CLI's own sign-in inside your workspace">
              <Globe className="size-4" /> Sign in with browser
            </Button>
            <Button size="sm" variant="secondary" onPress={onToken} tooltip="Paste a token or API key; it's kept in Vaultix">
              <KeyRound className="size-4" /> Use token/API key
            </Button>
          </>
        )}
      </div>
    </Card>
  );
}
