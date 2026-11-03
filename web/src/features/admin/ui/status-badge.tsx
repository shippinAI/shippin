import { Loader2 } from "lucide-react";
import { Badge, type BadgeTone } from "../../../lib/design/components/badge/badge";
import { StatusDot } from "../../../lib/design/components/status-dot/status-dot";

const map: Record<string, [string, BadgeTone]> = {
  active: ["Active", "success"], running: ["Running", "success"], suspended: ["Suspended", "warning"],
  provisioning: ["Provisioning", "info"], failed: ["Failed", "danger"], deleted: ["Deleted", "neutral"],
  stopped: ["Stopped", "neutral"], starting: ["Starting", "info"], stopping: ["Stopping", "warning"],
  deleting: ["Deleting", "warning"], user_state_active: ["Active", "success"], user_state_initial: ["Invited", "info"],
  user_state_inactive: ["Inactive", "neutral"], user_state_locked: ["Locked", "danger"],
  idp_state_active: ["Active", "success"], app_state_active: ["Active", "success"],
};
const busy = new Set(["provisioning", "starting", "stopping", "deleting"]);

/** Status → coloured badge for tenants, workspaces, users, IdPs and apps. */
export function StatusBadge({ status }: { status: string }) {
  const key = status.toLowerCase();
  const [label, tone] = map[key] ?? [status.replace(/^[A-Z_]+_STATE_/, "").toLowerCase(), "neutral" as BadgeTone];
  return (
    <Badge tone={tone}>
      {busy.has(key) ? (
        <Loader2 className="size-3 animate-spin motion-reduce:animate-none" />
      ) : (
        <StatusDot health={tone === "success" ? "up" : tone === "danger" ? "down" : "unknown"} />
      )}
      {label}
    </Badge>
  );
}
