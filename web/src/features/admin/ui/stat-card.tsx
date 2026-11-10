import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";

type Props = { label: string; value?: ReactNode; hint?: ReactNode; icon: LucideIcon; hue: string; pct?: number };

/** Dense stat card with optional usage bar (DO/Hetzner style). */
export function StatCard({ label, value, hint, icon: Icon, hue, pct }: Props) {
  const tone = pct === undefined ? hue : pct > 90 ? "var(--color-danger)" : pct > 75 ? "var(--color-warning)" : hue;
  return (
    <div className="rounded-xl border border-line bg-surface p-4 shadow-e1">
      <div className="flex items-center justify-between text-sm text-fg-muted">
        {label}
        <Icon className="size-4" style={{ color: hue }} strokeWidth={1.75} />
      </div>
      <div className="mt-2.5 text-2xl leading-none font-semibold tracking-tight tabular-nums">
        {value === undefined ? <Skeleton className="h-6 w-16" /> : value}
      </div>
      {pct !== undefined && (
        <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-raised" role="meter" aria-valuenow={Math.round(pct)} aria-valuemin={0} aria-valuemax={100} aria-label={label}>
          <div className="h-full rounded-full transition-[width]" style={{ width: `${Math.min(100, pct)}%`, background: tone }} />
        </div>
      )}
      {hint && <div className="mt-2 text-xs text-fg-subtle">{hint}</div>}
    </div>
  );
}
