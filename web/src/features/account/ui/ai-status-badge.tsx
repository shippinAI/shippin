import { Badge, type BadgeTone } from "../../../lib/design/components/badge/badge";
import type { ConnectionStatus } from "../../../lib/api/subscription-types";

const look: Record<ConnectionStatus, [BadgeTone, string]> = {
  not_connected: ["neutral", "Not connected"],
  activating: ["info", "Signing in…"],
  connected: ["success", "Connected"],
  needs_reauth: ["warning", "Sign in again"],
  limited: ["warning", "Sign-in unverified"],
  disconnected: ["neutral", "Signed out"],
};

export function AIStatusBadge({ status }: { status: ConnectionStatus }) {
  const [tone, label] = look[status];
  return <Badge tone={tone}>{label}</Badge>;
}
