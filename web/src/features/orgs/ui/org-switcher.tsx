import { Building2, Check, ChevronsUpDown, ShieldHalf } from "lucide-react";
import { Button } from "react-aria-components";
import { useLocation, useNavigate } from "../../../lib/router/router";
import { Menu, MenuItem } from "../../../lib/design/components/menu/menu";
import { cn } from "../../../lib/design/components/cn/cn";
import { useOrgs } from "../../../lib/hooks/use-orgs";
import { activeOrg, orgHome } from "../active-org";

/** Header org switcher: one Nomen identity, Operations (staff) and customer orgs side by side. */
export function OrgSwitcher() {
  const q = useOrgs();
  const nav = useNavigate();
  const loc = useLocation();
  const items = q.data?.items ?? [];
  if (items.length === 0) return null; // feature off, or no org yet
  const cur = activeOrg(items, loc.pathname);
  const ops = cur?.type === "operations";
  return (
    <Menu label="Switch organization" trigger={
      <Button data-testid="org-switcher" aria-label={`Organization: ${cur?.name ?? "none"}`}
        className={cn("focus-ring inline-flex h-8 max-w-[44vw] items-center gap-1.5 rounded-lg border px-2.5 text-xs font-medium outline-none",
          ops ? "border-warning/50 bg-warning/15 text-warning" : "border-line bg-raised text-fg")}>
        {ops ? <ShieldHalf className="size-3.5 shrink-0" aria-hidden /> : <Building2 className="size-3.5 shrink-0" aria-hidden />}
        <span className="truncate">{cur?.name ?? "Choose org"}</span>
        {ops && <span className="hidden rounded bg-warning/25 px-1 text-[10px] uppercase sm:inline">staff</span>}
        <ChevronsUpDown className="size-3.5 shrink-0 opacity-70" aria-hidden />
      </Button>}>
      {items.map((o) => (
        <MenuItem key={o.id} id={o.id} textValue={o.name} onAction={() => nav(orgHome(o))}>
          {o.type === "operations" ? <ShieldHalf className="size-4 text-warning" aria-hidden /> : <Building2 className="size-4" aria-hidden />}
          <span className="flex-1">
            <span className="block text-fg">{o.name}</span>
            <span className="block text-[11px] text-fg-subtle">{o.type === "operations" ? `Operations · ${o.roles.join(", ")}` : `Customer · ${o.roles.join(", ")}`}</span>
          </span>
          {cur?.id === o.id && <Check className="size-4" aria-label="current" />}
        </MenuItem>
      ))}
    </Menu>
  );
}
