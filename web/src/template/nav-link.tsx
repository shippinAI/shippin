import { Link } from "react-aria-components";
import { useLocation } from "../lib/router/router";
import { Tooltip } from "../lib/design/components/tooltip/tooltip";
import { cn } from "../lib/design/components/cn/cn";
import type { NavItem } from "./nav-items";

/** Sidebar entry; "soon" items are disabled with a tooltip saying why. */
export function NavLink({ item, href }: { item: NavItem; href?: string }) {
  const { pathname } = useLocation();
  const active = !!href && (pathname === href || ((item.prefix || href.split("/").length > 3) && pathname.startsWith(`${href}/`)));
  const Icon = item.icon;
  return (
    <Tooltip content={item.soon ?? `Open ${item.label}`} placement="right">
      <Link
        href={href}
        isDisabled={!href}
        aria-current={active ? "page" : undefined}
        className={cn(
          "focus-ring group relative flex h-8.5 cursor-default items-center gap-2.5 rounded-lg px-2.5 text-sm transition-colors",
          active ? "bg-raised text-fg shadow-e1" : "text-fg-muted data-hovered:bg-raised/60 data-hovered:text-fg",
          !href && "opacity-50",
        )}
      >
        {active && <span className="absolute top-2 bottom-2 -left-3 w-0.5 rounded-full" style={{ background: item.hue }} />}
        <Icon className="size-4 shrink-0" strokeWidth={1.75} style={{ color: active ? item.hue : undefined }} />
        <span className="flex-1 truncate">{item.label}</span>
        {item.soon && <span className="text-2xs font-medium tracking-wide text-fg-subtle uppercase">Soon</span>}
      </Link>
    </Tooltip>
  );
}
