import type { ReactNode } from "react";

type Props = { title: string; description: ReactNode; illustration?: ReactNode; action?: ReactNode };

/** Illustrated empty state (Flat Vector 2.0). */
export function EmptyState({ title, description, illustration, action }: Props) {
  return (
    <div className="flex flex-col items-center px-6 py-14 text-center">
      <div className="mb-5">{illustration ?? <DefaultArt />}</div>
      <h3 className="text-[15px] font-semibold text-fg">{title}</h3>
      <p className="mt-1.5 max-w-sm text-sm text-fg-muted">{description}</p>
      {action && <div className="mt-5">{action}</div>}
    </div>
  );
}

function DefaultArt() {
  return (
    <svg width="120" height="84" viewBox="0 0 120 84" fill="none" aria-hidden>
      <rect x="18" y="22" width="84" height="54" rx="10" fill="var(--color-raised)" stroke="var(--color-line-strong)" />
      <rect x="28" y="12" width="64" height="44" rx="9" fill="var(--color-overlay)" stroke="var(--color-line-strong)" />
      <rect x="38" y="24" width="30" height="5" rx="2.5" fill="var(--color-primary)" opacity=".8" />
      <rect x="38" y="34" width="44" height="4" rx="2" fill="var(--color-line-strong)" />
      <rect x="38" y="42" width="22" height="4" rx="2" fill="var(--color-line-strong)" />
      <circle cx="96" cy="18" r="7" fill="var(--color-hue-identity)" opacity=".85" />
    </svg>
  );
}
