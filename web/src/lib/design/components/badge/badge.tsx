import type { ReactNode } from "react";
import { cn } from "../cn/cn";

export type BadgeTone = "neutral" | "success" | "warning" | "danger" | "info" | "primary";

const tones: Record<BadgeTone, string> = {
  neutral: "bg-raised text-fg-muted border-line",
  success: "bg-success/12 text-success border-success/25",
  warning: "bg-warning/12 text-warning border-warning/25",
  danger: "bg-danger/12 text-danger border-danger/25",
  info: "bg-info/12 text-info border-info/25",
  primary: "bg-primary/12 text-primary border-primary/25",
};

export function Badge({ tone = "neutral", children, className }: { tone?: BadgeTone; children: ReactNode; className?: string }) {
  return (
    <span className={cn("inline-flex h-5.5 items-center whitespace-nowrap gap-1.5 rounded-full border px-2 text-[11.5px] font-medium", tones[tone], className)}>
      {children}
    </span>
  );
}
