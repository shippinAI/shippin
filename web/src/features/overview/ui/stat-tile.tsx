import type { LucideIcon } from "lucide-react";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";

type Props = { label: string; value?: string | number; hint: string; icon: LucideIcon; hue: string };

export function StatTile({ label, value, hint, icon: Icon, hue }: Props) {
  return (
    <div className="rounded-[var(--radius-card)] border border-line bg-surface p-4.5 shadow-[var(--shadow-e1)]">
      <div className="flex items-center justify-between text-[13px] text-fg-muted">
        {label}
        <Icon className="size-4" style={{ color: hue }} strokeWidth={1.75} />
      </div>
      <div className="mt-3 text-[28px] leading-none font-semibold tracking-tight tabular-nums">
        {value === undefined ? <Skeleton className="h-7 w-14" /> : value}
      </div>
      <div className="mt-2 text-xs text-fg-subtle">{hint}</div>
    </div>
  );
}
