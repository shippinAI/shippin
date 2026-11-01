import type { ReactElement, ReactNode } from "react";
import { Dialog, DialogTrigger, Heading, OverlayArrow, Popover as RacPopover } from "react-aria-components";

type Props = {
  /** A focusable RAC trigger (Button / IconButton). */
  trigger: ReactElement;
  title?: string;
  children: ReactNode;
  placement?: "top" | "bottom" | "left" | "right" | "bottom start" | "bottom end";
};

/** Click-to-open popover (RAC DialogTrigger + Popover + Dialog): focus moves in, Esc closes. */
export function Popover({ trigger, title, children, placement = "bottom" }: Props) {
  return (
    <DialogTrigger>
      {trigger}
      <RacPopover placement={placement} offset={10}
        className="glass z-50 w-80 max-w-[90vw] rounded-xl border border-line-strong p-4 text-[13px] text-fg-muted shadow-[var(--shadow-e3)] outline-none data-entering:animate-in data-entering:fade-in">
        <OverlayArrow>
          <svg width={10} height={10} viewBox="0 0 10 10" className="fill-overlay group-data-[placement=bottom]:rotate-180">
            <path d="M0 0 L5 5 L10 0" />
          </svg>
        </OverlayArrow>
        <Dialog className="space-y-2 outline-none" aria-label={title ? undefined : "Details"}>
          {title && <Heading slot="title" className="text-[13.5px] font-semibold text-fg">{title}</Heading>}
          {children}
        </Dialog>
      </RacPopover>
    </DialogTrigger>
  );
}
