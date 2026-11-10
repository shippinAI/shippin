import type { ReactNode } from "react";
import { Switch as RacSwitch, type SwitchProps } from "react-aria-components";
import { Tooltip } from "../tooltip/tooltip";

type Props = Omit<SwitchProps, "children" | "className"> & { children?: ReactNode; tooltip: ReactNode };

/** Toggle switch (RAC) with a required tooltip. */
export function Switch({ children, tooltip, ...rest }: Props) {
  return (
    <Tooltip content={tooltip}>
      <RacSwitch {...rest} className="group focus-ring inline-flex cursor-default items-center gap-2 rounded-full text-sm text-fg-muted data-disabled:opacity-50">
        <span className="flex h-5 w-9 items-center rounded-full border border-line-control bg-raised px-0.5 transition-colors group-data-selected:border-primary group-data-selected:bg-primary">
          <span className="size-3.5 rounded-full bg-fg-muted shadow-e1 transition-transform group-data-selected:translate-x-4 group-data-selected:bg-primary-fg motion-reduce:transition-none" />
        </span>
        {children}
      </RacSwitch>
    </Tooltip>
  );
}
