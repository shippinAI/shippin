import { useMe, usePublicSettings } from "../lib/hooks/use-api";
import { useTenant } from "../lib/hooks/use-tenant";
import { HelpPopover } from "../lib/design/components/popover/help-popover";
import { BrandMark } from "./brand-mark";
import { NavLink } from "./nav-link";
import { accountNav, operatorNav, tenantNav } from "./nav-items";
import { TenantSwitcher } from "./tenant-switcher";
import { UserMenu } from "./user-menu";

export function Sidebar() {
  const { tenant, isOperator } = useTenant();
  const settings = usePublicSettings().data;
  const caps = useMe().data?.capabilities ?? {};
  const base = tenant ? `/t/${tenant.tenant_id}` : undefined;
  return (
    <aside className="hidden w-64 md:flex shrink-0 flex-col border-r border-line bg-surface/60">
      <div className="flex h-14 items-center gap-2.5 px-5">
        <BrandMark size={26} />
        <span className="text-[15px] font-semibold tracking-tight">{settings?.brand.name ?? ""}</span>
      </div>
      <div className="px-3 pb-3">
        <TenantSwitcher />
      </div>
      <nav className="flex-1 space-y-5 overflow-y-auto px-3 pt-1" aria-label="Main">
        <div className="space-y-0.5">
          <div className="flex items-center justify-between px-2.5 pb-1">
            <span className="text-[11px] font-medium tracking-wider text-fg-subtle uppercase">Devspace</span>
            <HelpPopover title="Getting around" label="About the sidebar" placement="right">
              <p>Everything for this devspace lives here: workspaces, projects and the terminal.</p>
              <p>Hover any entry for a hint. Items marked <em>Soon</em> are not available yet.</p>
              <p>The chat button (bottom right) opens Clyffy on every page; it knows which page you're on.</p>
            </HelpPopover>
          </div>
          {tenantNav.map((item) => (
            <NavLink key={item.label} item={item} href={item.to !== undefined && base ? `${base}${item.to ? `/${item.to}` : ""}` : undefined} />
          ))}
        </div>
        <div className="space-y-0.5">
          <div className="px-2.5 pb-1.5 text-[11px] font-medium tracking-wider text-fg-subtle uppercase">Account</div>
          {accountNav.map((item) => (
            <NavLink key={item.label} item={item} href={item.to} />
          ))}
        </div>
        {isOperator && (
          <div className="space-y-0.5">
            <div className="px-2.5 pb-1.5 text-[11px] font-medium tracking-wider text-fg-subtle uppercase">Operator</div>
            {operatorNav.filter((i) => !i.cap || caps[i.cap]).map((item) => (
              <NavLink key={item.label} item={item} href={item.to} />
            ))}
          </div>
        )}
      </nav>
      <UserMenu area="tenant" />
    </aside>
  );
}
