import { Bot, LogOut, Plug } from "lucide-react";
import { useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { ConfirmDialog } from "../../../lib/design/components/dialog/confirm-dialog";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import type { AIProvider } from "../../../lib/api/subscription-types";
import { timeAgo } from "../../../lib/format";
import { useActivate, useDisconnect, useSubscriptions } from "../../../lib/hooks/use-subscriptions";
import { useTenant } from "../../../lib/hooks/use-tenant";
import { PageHeader } from "../../../template/page-header";
import { AIStatusBadge } from "./ai-status-badge";
import { AITerminalModal } from "./ai-terminal-modal";

/** Account → AI subscriptions: each user connects their OWN Claude Code / Codex / Grok plans. */
export function AIConnectionsPage() {
  const { tenant } = useTenant();
  const tid = tenant?.tenant_id ?? "";
  const q = useSubscriptions();
  const activate = useActivate(tid);
  const disconnect = useDisconnect(tid);
  const [active, setActive] = useState<AIProvider | null>(null);
  const [leaving, setLeaving] = useState<AIProvider | null>(null);
  const data = q.data;
  const home = data?.homes.find((h) => h.tenant_id === tid);
  const status = (p: string) => data?.connections.find((c) => c.provider === p && c.home_id === home?.id);
  const start = (p: AIProvider) => activate.mutate(p.id, { onSuccess: () => setActive(p) });
  return (
    <>
      <PageHeader eyebrow="Account" title="AI subscriptions"
        description="Connect your own Claude, ChatGPT/Codex and Grok plans on a private runner. Sign in with the official CLI and your local browser. The central Devagotchi.com hub will connect approved runners to your devspace; that integration is not available yet." />
      <Card elevation={2} className="overflow-hidden">
        <CardHeader title={tenant ? `In ${tenant.name}` : "AI subscriptions"} description={data?.terms_note} />
        {!data?.enabled ? (
          <EmptyState title="Not available yet" description="AI subscriptions aren't switched on for this cloud yet." />
        ) : (
          <ul className="divide-y divide-line">
            {data.providers.map((p) => {
              const c = status(p.id);
              const connected = c?.status === "connected" || c?.status === "needs_reauth";
              return (
                <li key={p.id} className="flex items-center gap-4 px-5 py-4">
                  <Bot className="size-5 text-hue-ai" aria-hidden />
                  <div className="min-w-0 flex-1">
                    <div className="text-sm font-medium text-fg">{p.label}</div>
                    <div className="text-xs text-fg-subtle">{p.vendor}{c?.checked_at ? ` · checked ${timeAgo(c.checked_at)}` : ""}</div>
                  </div>
                  <AIStatusBadge status={c?.status ?? "not_connected"} />
                  <Button size="sm" variant={connected ? "secondary" : "primary"} isDisabled={!tid || activate.isPending}
                    tooltip="Opens your private terminal with the vendor's sign-in command" onPress={() => start(p)}>
                    <Plug className="size-4" /> {connected ? "Reconnect" : "Connect"}
                  </Button>
                  {connected && (
                    <Button size="sm" variant="ghost" tooltip="Runs the vendor's own sign-out in your terminal" onPress={() => setLeaving(p)}>
                      <LogOut className="size-4" /> Sign out
                    </Button>
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </Card>
      <AITerminalModal tenantId={tid} provider={active} home={home} touchSeconds={data?.touch_seconds ?? 0} onClose={() => setActive(null)} />
      <ConfirmDialog
        isOpen={!!leaving}
        onOpenChange={(o) => !o && setLeaving(null)}
        title={`Sign out of ${leaving?.label}?`}
        body={<p>Your private runner signs out of this provider until you connect it again.</p>}
        confirmLabel="Sign out"
        onConfirm={() => leaving && disconnect.mutate(leaving.id)}
      />
    </>
  );
}
