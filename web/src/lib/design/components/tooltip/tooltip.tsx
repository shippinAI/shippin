import type { ReactElement, ReactNode } from "react";
import { OverlayArrow, Tooltip as RacTooltip, TooltipTrigger } from "react-aria-components";

type Props = {
  content: ReactNode;
  children: ReactElement;
  placement?: "top" | "bottom" | "left" | "right";
  delay?: number;
};

/** Accessible tooltip (RAC). Wrap any focusable RAC control. */
export function Tooltip({ content, children, placement = "top", delay = 400 }: Props) {
  return (
    <TooltipTrigger delay={delay} closeDelay={80}>
      {children}
      <RacTooltip
        placement={placement}
        offset={8}
        className="glass z-50 max-w-64 rounded-md border border-line-strong px-2.5 py-1.5 text-xs text-fg shadow-[var(--shadow-e3)] data-entering:animate-in data-entering:fade-in data-[placement=top]:data-entering:slide-in-from-bottom-1"
      >
        <OverlayArrow>
          <svg width={8} height={8} viewBox="0 0 8 8" className="fill-overlay group-data-[placement=bottom]:rotate-180">
            <path d="M0 0 L4 4 L8 0" />
          </svg>
        </OverlayArrow>
        {content}
      </RacTooltip>
    </TooltipTrigger>
  );
}
