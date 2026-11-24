import { AlertTriangle, Ban, Check, Clock, Minus, Play, X, type LucideIcon } from "lucide-react";
import type { RunnerJob } from "../../../../lib/api/actions.gen";
import { Badge } from "../../../../lib/design/components/badge/badge";
import { jobStatusView, refusalText, type StatusIcon } from "../model";

const icons: Record<StatusIcon, LucideIcon> = { clock: Clock, ban: Ban, play: Play, check: Check, x: X, minus: Minus };

/** A job's status: words, a token tone and an icon, so colour is never the only signal. */
export function JobStatusBadge({ job }: { job: Pick<RunnerJob, "status" | "conclusion"> }) {
  const v = jobStatusView(job);
  const Icon = icons[v.icon];
  return (
    <Badge tone={v.tone}>
      <Icon className="size-3" aria-hidden />
      {v.label}
    </Badge>
  );
}

/** Why a refused job got no runner, said plainly where the job is listed. */
export function Refusal({ job, compact }: { job: Pick<RunnerJob, "status" | "reason">; compact?: boolean }) {
  const text = refusalText(job);
  if (!text) return null;
  return (
    <p
      className={`flex items-start gap-1.5 text-danger ${compact ? "mt-1 text-sm" : "rounded-lg border border-danger/25 bg-danger/12 p-3 text-base"}`}
    >
      <AlertTriangle className="mt-0.5 size-4 shrink-0" aria-hidden />
      <span>
        <span className="sr-only">Refused: </span>
        {text}
      </span>
    </p>
  );
}
