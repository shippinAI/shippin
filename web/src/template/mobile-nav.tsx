import { Cpu, Database, KeyRound, LayoutGrid, Menu as MenuIcon, MessageSquare, ShieldCheck, Boxes } from "lucide-react";
import { Button } from "react-aria-components";
import { Menu, MenuItem } from "../lib/design/components/menu/menu";
import { useMe } from "../lib/hooks/use-api";
import { useParams } from "../lib/router/router";

/** Below md the sidebar is hidden; this menu is the phone navigation, Gold approvals first. */
export function MobileNav() {
  const me = useMe().data;
  const t = useParams().tenantId ?? me?.tenants[0]?.tenant_id;
  const icon = "size-4";
  return (
    <div className="md:hidden">
      <Menu label="Navigation"
        trigger={
          <Button aria-label="Open navigation" data-testid="mobile-nav"
            className="focus-ring inline-flex size-9 items-center justify-center rounded-lg text-fg-muted outline-none data-pressed:bg-raised">
            <MenuIcon className="size-5" aria-hidden />
          </Button>
        }>
        <MenuItem href="/gold"><ShieldCheck className={icon} aria-hidden /> Gold approvals</MenuItem>
        <MenuItem href="/gold/passkeys"><KeyRound className={icon} aria-hidden /> Passkeys</MenuItem>
        {t && <MenuItem href={`/t/${t}`}><LayoutGrid className={icon} aria-hidden /> Home</MenuItem>}
        {t && <MenuItem href={`/t/${t}/workspaces`}><Boxes className={icon} aria-hidden /> Workspaces</MenuItem>}
        {t && <MenuItem href={`/t/${t}/database`}><Database className={icon} aria-hidden /> Database</MenuItem>}
        <MenuItem href="/chat"><MessageSquare className={icon} aria-hidden /> Automaton chat</MenuItem>
        {me?.capabilities["infra.hosts"] && <MenuItem href="/operator/hosts"><Cpu className={icon} aria-hidden /> Hosts</MenuItem>}
      </Menu>
    </div>
  );
}
