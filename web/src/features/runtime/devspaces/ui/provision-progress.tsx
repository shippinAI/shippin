import { Check, Circle, Loader2 } from "lucide-react";
import { Card, CardHeader } from "../../../../lib/design/components/card/card";
import type { Workspace } from "../../../../lib/api/types";
import { cn } from "../../../../lib/design/components/cn/cn";

/** The steps of making a development space, drawn as the server lists them. */
export function ProvisionProgress({ ws }: { ws: Workspace }) {
  const steps = ws.steps;
  const done = steps.filter((s) => s.state === "done").length;
  const running = steps.some((s) => s.state === "running") ? 0.5 : 0;
  return (
    <Card className="mb-6">
      <CardHeader
        title="Setting up your workspace"
        description="This usually takes under a minute; first use of a template can take longer while the image is copied."
      />
      <ol className="space-y-2.5 px-5 py-4" aria-live="polite">
        {steps.map((s) => (
          <li
            key={s.id}
            className={cn(
              "flex items-center gap-3 text-base",
              s.state === "done" ? "text-fg-muted" : s.state === "running" ? "text-fg" : "text-fg-subtle",
            )}
          >
            {s.state === "done" ? (
              <Check className="size-4 text-success" />
            ) : s.state === "running" ? (
              <Loader2 className="size-4 animate-spin text-primary motion-reduce:animate-none" />
            ) : (
              <Circle className="size-4" />
            )}
            {s.label}
          </li>
        ))}
      </ol>
      <div className="h-1 overflow-hidden rounded-b-xl bg-raised">
        <div
          className="h-full bg-primary transition-[width] duration-700"
          style={{ width: `${steps.length ? ((done + running) / steps.length) * 100 : 0}%` }}
        />
      </div>
    </Card>
  );
}
