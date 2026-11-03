import { Menu as MenuIcon } from "lucide-react";
import { Button } from "react-aria-components";
import { Menu, MenuItem } from "../lib/design/components/menu/menu";
import { useMe } from "../lib/hooks/use-api";
import { useParams } from "../lib/router/router";
import { applyNav } from "../lib/ui-config/model";
import { useUiConfig } from "../lib/ui-config/use-ui-config";
import { accountNav, allowedBy, infrastructureNav, tenantNav } from "./nav-items";

/** Below md the sidebar is hidden; this menu is the phone navigation, the same entries it configures. */
export function MobileNav() {
  const me = useMe().data;
  const overrides = useUiConfig().data?.nav?.items;
  const t = useParams().tenantId ?? me?.tenants[0]?.tenant_id;
  const allowed = allowedBy(me?.capabilities ?? {});
  const icon = "size-4";
  const isTenant = (id: string) => tenantNav.some((n) => n.id === id);
  const tenantHref = (to: string) => (to.startsWith("/") ? to : `/t/${t}${to ? `/${to}` : ""}`);
  const entries = applyNav([...accountNav, ...tenantNav, ...infrastructureNav].filter((i) => i.mobile && i.to !== undefined && allowed(i)), overrides)
    .filter((i) => t || !isTenant(i.id))
    .map((i) => ({ ...i, href: isTenant(i.id) ? tenantHref(i.to ?? "") : i.to }));
  return (
    <div className="md:hidden">
      <Menu label="Navigation"
        trigger={
          <Button aria-label="Open navigation" data-testid="mobile-nav"
            className="focus-ring inline-flex size-9 items-center justify-center rounded-lg text-fg-muted outline-none data-pressed:bg-raised">
            <MenuIcon className="size-5" aria-hidden />
          </Button>
        }>
        {entries.map((i) => (
          <MenuItem key={i.id} href={i.href}><i.icon className={icon} aria-hidden /> {i.label}</MenuItem>
        ))}
      </Menu>
    </div>
  );
}
