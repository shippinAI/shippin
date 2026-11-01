import { cn } from "../cn/cn";

export type Health = "up" | "down" | "unknown";

const colors: Record<Health, string> = { up: "bg-success", down: "bg-danger", unknown: "bg-fg-subtle" };

/** Pulsing health indicator; motion is disabled for reduced-motion users. */
export function StatusDot({ health, className }: { health: Health; className?: string }) {
  return (
    <span className={cn("relative inline-flex size-2", className)} aria-hidden>
      {health === "up" && (
        <span className="absolute inset-0 animate-ping rounded-full bg-success/60 motion-reduce:hidden" />
      )}
      <span className={cn("relative inline-flex size-2 rounded-full", colors[health])} />
    </span>
  );
}
