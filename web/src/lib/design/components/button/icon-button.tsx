import type { LucideIcon } from "lucide-react";
import { Button as RacButton, type ButtonProps } from "react-aria-components";
import { Tooltip } from "../tooltip/tooltip";
import { cn } from "../cn/cn";

type Props = Omit<ButtonProps, "className" | "children"> & {
  icon: LucideIcon;
  label: string;
  className?: string;
};

/** Square icon button; its label is both the aria-label and the tooltip. */
export function IconButton({ icon: Icon, label, className, ...rest }: Props) {
  return (
    <Tooltip content={label}>
      <RacButton
        {...rest}
        aria-label={label}
        className={cn(
          "focus-ring inline-flex size-8 cursor-default items-center justify-center rounded-lg text-fg-muted transition-colors data-hovered:bg-raised data-hovered:text-fg data-pressed:bg-overlay data-disabled:opacity-40",
          className,
        )}
      >
        <Icon className="size-4" strokeWidth={1.75} />
      </RacButton>
    </Tooltip>
  );
}
