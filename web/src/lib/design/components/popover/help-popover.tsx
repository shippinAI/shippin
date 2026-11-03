import { CircleHelp } from "lucide-react";
import type { ReactNode } from "react";
import { Button } from "react-aria-components";
import { Tooltip } from "../tooltip/tooltip";
import { Popover } from "./popover";

type Props = { title: string; children: ReactNode; label?: string; placement?: "top" | "bottom" | "left" | "right" };

/** "?" button: hover/focus shows a tooltip, press opens a popover that explains the area. */
export function HelpPopover({ title, children, label, placement = "bottom" }: Props) {
  const name = label ?? `About ${title}`;
  return (
    <Popover title={title} placement={placement}
      trigger={
        <Tooltip content={name}>
          <Button aria-label={name}
            className="focus-ring inline-flex size-6 items-center justify-center rounded-full text-fg-subtle outline-none data-hovered:bg-raised data-hovered:text-fg data-pressed:text-fg">
            <CircleHelp className="size-4" aria-hidden />
          </Button>
        </Tooltip>
      }>
      {children}
    </Popover>
  );
}
