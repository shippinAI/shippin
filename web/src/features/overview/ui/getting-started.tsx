import { CheckCircle2, Circle } from "lucide-react";
import { Card, CardHeader } from "../../../lib/design/components/card/card";

type Step = { title: string; body: string; done: boolean };

/** Onboarding checklist; state is derived from what exists today. */
export function GettingStarted({ steps }: { steps: Step[] }) {
  const done = steps.filter((s) => s.done).length;
  return (
    <Card>
      <CardHeader
        title="Get set up"
        description={`${done} of ${steps.length} complete`}
        actions={
          <div className="h-1.5 w-28 overflow-hidden rounded-full bg-raised">
            <div className="h-full rounded-full bg-primary" style={{ width: `${(done / steps.length) * 100}%` }} />
          </div>
        }
      />
      <ol className="divide-y divide-line">
        {steps.map((s) => (
          <li key={s.title} className="flex gap-3 px-5 py-3.5">
            {s.done ? (
              <CheckCircle2 className="mt-0.5 size-4.5 shrink-0 text-success" />
            ) : (
              <Circle className="mt-0.5 size-4.5 shrink-0 text-fg-subtle" />
            )}
            <div>
              <div className={`text-base font-medium ${s.done ? "text-fg-muted line-through decoration-fg-subtle/50" : "text-fg"}`}>{s.title}</div>
              <div className="mt-0.5 text-sm text-fg-subtle">{s.body}</div>
            </div>
          </li>
        ))}
      </ol>
    </Card>
  );
}
