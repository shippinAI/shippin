import { Meter as RacMeter } from "react-aria-components";
import { cn } from "../cn/cn";
import { meterLevel, usedFraction } from "./meter-model";

export type MeterTone = "primary" | "success" | "data" | "workspaces";

const fills: Record<MeterTone | "warning" | "danger", string> = {
  primary: "bg-primary", success: "bg-success", data: "bg-hue-data", workspaces: "bg-hue-workspaces", warning: "bg-warning", danger: "bg-danger",
};

type Props = {
  /** What is measured; also the meter's accessible name. */
  label: string;
  used?: number | null;
  total?: number | null;
  /** The numbers as read, such as "3.1 GB / 8 GB" or "Not reported". */
  text: string;
  tone?: MeterTone;
  /** Turn amber above 75 % and red above 90 %. Off for counts where full is good, such as running guests. */
  pressure?: boolean;
  /** Draw only the bar; the label still names it for assistive technology. */
  bare?: boolean;
  size?: "sm" | "md";
  className?: string;
};

/** Used of total, as a bar with its numbers. An unknown total draws an empty track and no meter, never a zero. */
export function Meter({ label, used, total, text, tone = "primary", pressure = true, bare, size = "sm", className }: Props) {
  const fraction = usedFraction(used, total);
  const level = pressure ? meterLevel(fraction) : "normal";
  const fill = fills[level === "normal" ? tone : level];
  const track = cn("overflow-hidden rounded-full bg-raised", size === "sm" ? "h-1.5" : "h-2");
  const header = !bare && (
    <div className="mb-1 flex items-baseline justify-between gap-3 text-xs">
      <span className="truncate text-fg-muted">{label}</span>
      <span className="shrink-0 text-fg tabular-nums">{text}</span>
    </div>
  );
  if (fraction === null) {
    return (
      <div className={className}>
        {header}
        <div className={track} aria-hidden />
        {bare && <span className="sr-only">{label}: {text}</span>}
      </div>
    );
  }
  return (
    <RacMeter aria-label={label} value={fraction * 100} valueLabel={text} className={className}>
      {header}
      <div className={track}>
        <div className={cn("h-full rounded-full", fill)} style={{ width: `${fraction * 100}%` }} />
      </div>
    </RacMeter>
  );
}
