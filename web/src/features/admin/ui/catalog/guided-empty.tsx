import type { ReactNode } from "react";
import { EmptyState } from "../../../../lib/design/components/empty-state/empty-state";

type Props = { title: string; steps: string[]; action?: ReactNode };

/** Empty state that tells the operator what to do next, as numbered steps. */
export function GuidedEmpty({ title, steps, action }: Props) {
  return (
    <EmptyState
      title={title}
      action={action}
      description={
        <ol className="mt-1 list-decimal space-y-1 pl-5 text-left">
          {steps.map((s) => <li key={s}>{s}</li>)}
        </ol>
      }
    />
  );
}
