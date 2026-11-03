import type { ReactNode } from "react";

type Props = { eyebrow?: string; title: string; description?: ReactNode; actions?: ReactNode };

export function PageHeader({ eyebrow, title, description, actions }: Props) {
  return (
    <header className="mb-8 flex flex-wrap items-end justify-between gap-4">
      <div>
        {eyebrow && <div className="mb-1.5 text-xs font-medium tracking-wider text-primary uppercase">{eyebrow}</div>}
        <h1 className="text-[26px] font-semibold tracking-tight text-fg">{title}</h1>
        {description && <p className="mt-1.5 max-w-2xl text-sm text-fg-muted">{description}</p>}
      </div>
      {actions && <div className="flex items-center gap-2">{actions}</div>}
    </header>
  );
}
