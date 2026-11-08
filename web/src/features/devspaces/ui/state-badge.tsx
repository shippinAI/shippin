import { Loader2 } from "lucide-react";
import { Badge, type BadgeTone } from "../../../lib/design/components/badge/badge";
import { StatusDot } from "../../../lib/design/components/status-dot/status-dot";
import type { WorkspaceState } from "../../../lib/api/types";

const words: Record<WorkspaceState, [string, BadgeTone]> = {
  running: ["Running", "success"], stopped: ["Stopped", "neutral"], provisioning: ["Setting up", "info"],
  starting: ["Starting", "info"], stopping: ["Stopping", "warning"], deleting: ["Deleting", "warning"],
  failed: ["Needs attention", "danger"], unknown: ["Status unavailable", "warning"],
};

export function StateBadge({ state }: { state: WorkspaceState }) {
  const [label, tone] = words[state] ?? [state, "neutral"];
  const busy = ["provisioning", "starting", "stopping", "deleting"].includes(state);
  return (
    <Badge tone={tone}>
      {busy ? <Loader2 className="size-3 animate-spin" /> : <StatusDot health={state === "running" ? "up" : state === "failed" ? "down" : "unknown"} />}
      {label}
    </Badge>
  );
}
