import type { ReactNode } from "react";
import { Menu as RacMenu, MenuItem as RacMenuItem, MenuTrigger, Popover, type MenuItemProps } from "react-aria-components";

type Props = { trigger: ReactNode; children: ReactNode; label: string };

/** Popover menu; the trigger must be a RAC Button. */
export function Menu({ trigger, children, label }: Props) {
  return (
    <MenuTrigger>
      {trigger}
      <Popover
        placement="bottom start"
        offset={6}
        className="bg-overlay min-w-60 rounded-xl border border-line-strong p-1 shadow-e3 outline-none"
      >
        <RacMenu aria-label={label} className="outline-none">
          {children}
        </RacMenu>
      </Popover>
    </MenuTrigger>
  );
}

export function MenuItem(props: MenuItemProps & { children: ReactNode }) {
  return (
    <RacMenuItem
      {...props}
      className="flex cursor-default items-center gap-2.5 rounded-lg px-2.5 py-2 text-base text-fg-muted outline-none data-focused:bg-raised data-focused:text-fg data-disabled:opacity-50"
    />
  );
}
