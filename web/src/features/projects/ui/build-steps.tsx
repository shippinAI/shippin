import { Check, CircleDashed, Loader2, X } from "lucide-react";
import type { RunState } from "../../../lib/api/build-types";

/** Index of the step each run state is on (5 = all done). */
const current: Record<RunState, number> = { queued: 1, provisioning: 1, waiting_ai: 2, starting: 3, running: 3, exited: 5, failed: 1 };

/** Where the build is, in plain words (M4 job steps: devspace → sign-in → agent → done). */
export function BuildSteps({ state, label }: { state: RunState; label: string }) {
  const at = current[state] ?? 0;
  const steps = ["Project created", "Devspace ready on your private server", `Signed in to ${label}`, `${label} is building`, "Finished"];
  return (
    <ol className="space-y-2.5">
      {steps.map((text, i) => {
        const done = i < at;
        const now = i === at;
        const failed = now && state === "failed";
        return (
          <li key={text} className={`flex items-center gap-2.5 text-base ${done || now ? "text-fg" : "text-fg-subtle"}`}>
            {done && <Check className="size-4 text-success" />}
            {now && !failed && <Loader2 className="size-4 animate-spin text-primary" />}
            {failed && <X className="size-4 text-danger" />}
            {!done && !now && <CircleDashed className="size-4" />}
            {text}
          </li>
        );
      })}
    </ol>
  );
}
