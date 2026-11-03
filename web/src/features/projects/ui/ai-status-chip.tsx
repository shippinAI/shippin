import { Bot } from "lucide-react";
import { useNavigate } from "../../../lib/router/router";
import { Button } from "../../../lib/design/components/button/button";
import { useSubscriptions } from "../../../lib/hooks/use-subscriptions";
import { useTenant } from "../../../lib/hooks/use-tenant";

/** Devspace chip: how many of my own AI subscriptions this tenant's workspaces can use. */
export function AIStatusChip() {
  const tenantId = useTenant().tenant?.tenant_id;
  const q = useSubscriptions();
  const nav = useNavigate();
  if (!q.data?.enabled) return null;
  const home = q.data.homes.find((h) => h.tenant_id === tenantId);
  const n = q.data.connections.filter((c) => c.home_id === home?.id && c.status === "connected").length;
  return (
    <Button variant="ghost" size="sm" onPress={() => nav("/account/ai")}
      tooltip={n ? "Your own AI subscriptions are available in your workspaces here" : "Connect your own Claude, Codex or Grok subscription"}>
      <Bot className="size-4" /> {n ? `AI: ${n} connected` : "Connect AI"}
    </Button>
  );
}
