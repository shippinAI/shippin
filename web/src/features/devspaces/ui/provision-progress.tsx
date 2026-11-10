import { Check, Circle, Loader2 } from "lucide-react";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import type { Workspace } from "../../../lib/api/types";
import { cn } from "../../../lib/design/components/cn/cn";

// Mirrors the steps shippin-hostd reports (matched by prefix).
const base = ["Creating home volume", "Launching", "Starting", "Waiting for network", "Installing SSH keys"];

/** Live provisioning checklist shown while a workspace is being created. */
export function ProvisionProgress({ ws }: { ws: Workspace }) {
  const steps = [...base, ...(ws.repo ? ["Cloning"] : []), ...(ws.template_id ? ["Running startup script"] : []), "Publishing HTTPS routes"];
  const cur = Math.max(0, steps.findIndex((s) => ws.progress?.startsWith(s)));
  return (
    <Card className="mb-6">
      <CardHeader title="Setting up your workspace" description="This usually takes under a minute; first use of a template can take longer while the image is copied." />
      <ol className="space-y-2.5 px-5 py-4" aria-live="polite">
        {steps.map((s, i) => {
          const done = i < cur, now = i === cur;
          return (
            <li key={s} className={cn("flex items-center gap-3 text-base", done ? "text-fg-muted" : now ? "text-fg" : "text-fg-subtle")}>
              {done ? <Check className="size-4 text-success" /> : now ? <Loader2 className="size-4 animate-spin text-primary motion-reduce:animate-none" /> : <Circle className="size-4" />}
              {now && ws.progress ? ws.progress : s}
            </li>
          );
        })}
      </ol>
      <div className="h-1 overflow-hidden rounded-b-xl bg-raised">
        <div className="h-full bg-primary transition-[width] duration-700" style={{ width: `${((cur + 0.5) / steps.length) * 100}%` }} />
      </div>
    </Card>
  );
}
