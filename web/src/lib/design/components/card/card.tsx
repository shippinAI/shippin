import type { ReactNode } from "react";
import { cn } from "../cn/cn";

type Props = { children: ReactNode; className?: string; elevation?: 1 | 2 | 3; "data-testid"?: string };

/** Elevated surface with a hairline highlight. */
export function Card({ children, className, elevation = 1, "data-testid": testId }: Props) {
  const shadow = { 1: "shadow-e1", 2: "shadow-e2", 3: "shadow-e3" }[elevation];
  return (
    <div data-testid={testId} className={cn("rounded-xl border border-line bg-surface", shadow, className)}>{children}</div>
  );
}

type HeaderProps = { title: ReactNode; description?: ReactNode; actions?: ReactNode };

export function CardHeader({ title, description, actions }: HeaderProps) {
  return (
    <div className="flex items-start justify-between gap-4 border-b border-line px-5 py-4">
      <div className="min-w-0">
        <h3 className="text-base font-semibold text-fg">{title}</h3>
        {description && <p className="mt-0.5 text-sm text-fg-muted">{description}</p>}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-1.5">{actions}</div>}
    </div>
  );
}
