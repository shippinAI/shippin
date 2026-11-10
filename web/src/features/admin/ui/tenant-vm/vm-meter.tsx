type Props = { label: string; used?: number; total?: number; text: string };

/** Compact labelled usage bar for the tenant VM card. */
export function VMMeter({ label, used, total, text }: Props) {
  const pct = used && total ? Math.min(100, (used / total) * 100) : undefined;
  const tone = pct === undefined ? "var(--color-hue-workspaces)" : pct > 90 ? "var(--color-danger)" : pct > 75 ? "var(--color-warning)" : "var(--color-hue-workspaces)";
  return (
    <div>
      <div className="flex items-baseline justify-between text-xs">
        <span className="text-fg-muted">{label}</span>
        <span className="font-mono text-fg tabular-nums">{text}</span>
      </div>
      <div
        className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-raised"
        role="meter"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={pct === undefined ? 0 : Math.round(pct)}
      >
        <div className="h-full rounded-full transition-[width]" style={{ width: `${pct ?? 0}%`, background: tone }} />
      </div>
    </div>
  );
}
