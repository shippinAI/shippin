import { SquareTerminal } from "lucide-react";
import { useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { ConfirmDialog } from "../../../lib/design/components/dialog/confirm-dialog";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import type { Connection } from "../../../lib/api/connect-types";
import { useConnections, useDisconnectProvider, useStartLogin } from "../../../lib/hooks/use-connections";
import { useParams } from "../../../lib/router/router";
import { HelpPopover } from "../../../lib/design/components/popover/help-popover";
import { PageHeader } from "../../../template/page-header";
import { BrowserLoginDialog } from "./browser-login-dialog";
import { ProviderCard } from "./provider-card";
import { TokenDialog } from "./token-dialog";

/** Workspace → Connect accounts: GitHub, Claude, Codex and Grok, signed in once and kept. */
export function ConnectAccountsPage() {
  const { tenantId = "", workspaceId = "" } = useParams();
  const q = useConnections(tenantId, workspaceId);
  const start = useStartLogin(tenantId, workspaceId);
  const disconnect = useDisconnectProvider(tenantId, workspaceId);
  const [browser, setBrowser] = useState<Connection | null>(null);
  const [token, setToken] = useState<Connection | null>(null);
  const [leaving, setLeaving] = useState<Connection | null>(null);
  const d = q.data;
  const signIn = (c: Connection) => start.mutate(c.id, { onSuccess: () => setBrowser(c) });
  return (
    <>
      <PageHeader eyebrow="Workspace" title="Connect accounts"
        description="Sign in once; the workspace's credential writer keeps every CLI signed in across restarts. Tokens are kept in Vaultix, never shown again."
        actions={<span className="flex items-center gap-2">
          <HelpPopover title="Browser sign-in or token?" placement="bottom">
            <p><strong className="text-fg">Sign in with browser</strong> runs the CLI's own device-code login in the workspace. Best for subscriptions (Claude Pro, ChatGPT).</p>
            <p><strong className="text-fg">Use token/API key</strong> stores a key in Vaultix and writes it into the workspace on every start. It is never shown again.</p>
          </HelpPopover>
          {d?.ide_terminal_url && (
          <Button variant="secondary" tooltip="Opens the workspace IDE at its terminal" onPress={() => window.open(d.ide_terminal_url, "_blank", "noopener")}>
            <SquareTerminal className="size-4" /> Open terminal
          </Button>
          )}
        </span>} />
      {d && !d.running && <p className="mb-4 text-sm text-warning">Start the workspace to sign in with a browser. Saved tokens are added when it starts.</p>}
      {q.isError ? (
        <EmptyState title="Not available" description={q.error.message} />
      ) : (
        <div className="grid gap-4 md:grid-cols-2">
          {d?.connections.map((c) => (
            <ProviderCard key={c.id} c={c} running={d.running} onBrowser={() => signIn(c)} onToken={() => setToken(c)} onDisconnect={() => setLeaving(c)} />
          ))}
        </div>
      )}
      <BrowserLoginDialog tenantId={tenantId} workspaceId={workspaceId} provider={browser} onClose={() => { setBrowser(null); void q.refetch(); }} />
      <TokenDialog tenantId={tenantId} workspaceId={workspaceId} provider={token} onClose={() => setToken(null)} />
      <ConfirmDialog isOpen={!!leaving} onOpenChange={(o) => !o && setLeaving(null)} title={`Disconnect ${leaving?.label}?`}
        body={<p>The CLI is signed out in this workspace and any saved token is removed from Vaultix.</p>}
        confirmLabel="Disconnect" onConfirm={() => leaving && disconnect.mutate(leaving.id)} />
    </>
  );
}
