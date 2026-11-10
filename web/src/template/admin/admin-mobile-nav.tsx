import { useMe } from "../../lib/hooks/use-api";
import { Link } from "react-aria-components";
import { useLocation } from "../../lib/router/router";
import { useAdminAccess } from "../../lib/hooks/use-admin";
import { applyNav } from "../../lib/ui-config/model";
import { useUiConfig } from "../../lib/ui-config/use-ui-config";
import { featureOn } from "../nav-items";
import { adminNav } from "./admin-nav";

/** Narrow screens: the provider sections as a horizontally scrolling tab strip. */
export function AdminMobileNav() {
  const perms = useAdminAccess().data?.staff?.permissions ?? [];
  const { pathname } = useLocation();
  const overrides = useUiConfig().data?.nav?.items;
  const caps = useMe().data?.capabilities ?? {};
  const items = applyNav(adminNav.filter(featureOn(caps)).filter((i) => !i.cap || i.cap.split("|").some((p) => perms.includes(p))), overrides);
  return (
    <nav aria-label="Provider panel" className="flex gap-1 overflow-x-auto border-b border-line bg-surface/80 px-3 py-2 md:hidden">
      {items.map((i) => (
        <Link key={i.to} href={i.to ?? "/admin"}
          className={`focus-ring flex shrink-0 items-center gap-1.5 rounded-md px-2.5 py-1.5 text-sm ${pathname.startsWith(i.to ?? "/admin") ? "bg-raised text-fg" : "text-fg-muted"}`}>
          <i.icon className="size-3.5" /> {i.label}
        </Link>
      ))}
    </nav>
  );
}
