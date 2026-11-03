import { ChevronsUpDown, KeySquare, LayoutGrid, LogOut, ShieldCheck } from "lucide-react";
import { Button, Separator } from "react-aria-components";
import { useNavigate } from "../lib/router/router";
import { Menu, MenuItem } from "../lib/design/components/menu/menu";
import { initials } from "../lib/format";
import { useTenant } from "../lib/hooks/use-tenant";
import { usePublicSettings } from "../lib/hooks/use-api";

/** Account menu at the foot of both sidebars: tenant console ↔ provider panel switch, keys, sign out.
 *  "Admin panel" appears only when the server grants provider.panel. */
export function UserMenu({ area }: { area: "tenant" | "admin" }) {
  const { me, tenant } = useTenant();
  const nav = useNavigate();
  const admin = me?.capabilities["provider.panel"] === true;
  const s = usePublicSettings().data;
  const ops = s?.operator_origin_only && s.operator_url ? s.operator_url : "";
  const toAdmin = () => {
    const path = `/admin/staff-mode?from=${encodeURIComponent(tenant?.tenant_id ?? "")}`;
    return ops ? window.location.assign(ops + path) : nav(path);
  };
  const trigger = (
    <Button aria-label="Account menu" className="focus-ring flex w-full cursor-default items-center gap-2.5 rounded-lg p-1.5 text-left data-hovered:bg-raised">
      <span className="flex size-8 items-center justify-center rounded-full bg-overlay text-xs font-semibold text-fg ring-1 ring-line-strong">
        {initials(me?.display_name ?? "")}
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[13px] font-medium">{me?.display_name}</span>
        <span className="block truncate text-[11px] text-fg-subtle">{me?.email}</span>
      </span>
      <ChevronsUpDown className="size-3.5 text-fg-subtle" />
    </Button>
  );
  return (
    <div className="border-t border-line p-2">
      <Menu trigger={trigger} label="Account">
        {area === "tenant" && admin && (
          <MenuItem id="admin" textValue="Admin panel" onAction={toAdmin}>
            <ShieldCheck className="size-4 text-warning" /> <span className="flex-1">Admin panel</span>
          </MenuItem>
        )}
        {area === "admin" && (
          <MenuItem id="ws" textValue="Back to my workspace" onAction={() => nav(tenant ? `/t/${tenant.tenant_id}/workspaces` : "/")}>
            <LayoutGrid className="size-4" /> <span className="flex-1">Back to my workspace</span>
          </MenuItem>
        )}
        <MenuItem id="keys" textValue="SSH keys" onAction={() => nav("/account/ssh-keys")}>
          <KeySquare className="size-4" /> SSH keys
        </MenuItem>
        <Separator className="my-1 border-t border-line" />
        <MenuItem id="out" textValue="Sign out" onAction={() => window.location.assign("/auth/logout")}>
          <LogOut className="size-4" /> Sign out
        </MenuItem>
      </Menu>
    </div>
  );
}
