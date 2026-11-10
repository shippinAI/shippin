import { useMe } from "../../lib/hooks/use-api";
import { ShieldCheck } from "lucide-react";
import { usePublicSettings } from "../../lib/hooks/use-api";
import { useAdminAccess } from "../../lib/hooks/use-admin";
import { applyNav } from "../../lib/ui-config/model";
import { useUiConfig } from "../../lib/ui-config/use-ui-config";
import { BrandMark } from "../brand-mark";
import { NavLink } from "../nav-link";
import { UserMenu } from "../user-menu";
import { WorkspaceSwitch } from "./workspace-switch";
import { featureOn } from "../nav-items";
import { adminNav } from "./admin-nav";

export function AdminSidebar() {
  const brand = usePublicSettings().data?.brand.name ?? "";
  const perms = useAdminAccess().data?.staff?.permissions ?? [];
  // Sections the caller can't use are hidden (the API refuses them anyway).
  const overrides = useUiConfig().data?.nav?.items;
  const caps = useMe().data?.capabilities ?? {};
  const items = applyNav(adminNav.filter(featureOn(caps)).filter((i) => !i.cap || i.cap.split("|").some((p) => perms.includes(p))), overrides);
  return (
    <aside className="hidden w-60 shrink-0 md:flex flex-col border-r border-line bg-surface/60">
      <div className="flex h-14 items-center gap-2.5 px-5">
        <BrandMark size={24} />
        <div className="leading-tight">
          <div className="text-base font-semibold tracking-tight">{brand}</div>
          <div className="flex items-center gap-1 text-2xs font-medium tracking-wider text-warning uppercase">
            <ShieldCheck className="size-3" /> Provider
          </div>
        </div>
      </div>
      <nav className="flex-1 space-y-0.5 overflow-y-auto px-3 pt-3" aria-label="Provider panel">
        {items.map((item) => (
          <NavLink key={item.id} item={item} href={item.to} />
        ))}
      </nav>
      <WorkspaceSwitch />
      <UserMenu area="admin" />
    </aside>
  );
}
