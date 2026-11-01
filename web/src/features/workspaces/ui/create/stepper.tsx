import { Check } from "lucide-react";
import { cn } from "../../../../lib/design/components/cn/cn";

/** Horizontal step indicator for the create flow. */
export function Stepper({ steps, current }: { steps: string[]; current: number }) {
  return (
    <ol className="mb-6 flex items-center gap-3" aria-label="Progress">
      {steps.map((s, i) => (
        <li key={s} className="flex items-center gap-3" aria-current={i === current ? "step" : undefined}>
          <span
            className={cn(
              "flex size-6 items-center justify-center rounded-full border text-xs font-semibold tabular-nums",
              i < current && "border-primary bg-primary text-white",
              i === current && "border-primary text-primary",
              i > current && "border-line-strong text-fg-subtle",
            )}
          >
            {i < current ? <Check className="size-3.5" strokeWidth={2.5} /> : i + 1}
          </span>
          <span className={cn("text-sm", i === current ? "font-medium text-fg" : "text-fg-muted")}>{s}</span>
          {i < steps.length - 1 && <span className="h-px w-10 bg-line-strong" />}
        </li>
      ))}
    </ol>
  );
}
