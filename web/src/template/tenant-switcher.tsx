import { Check, ChevronsUpDown } from "lucide-react";
import { Button } from "react-aria-components";
import { useNavigate } from "../lib/router/router";
import { Menu, MenuItem } from "../lib/design/components/menu/menu";
import { Tooltip } from "../lib/design/components/tooltip/tooltip";
import { toast } from "../lib/design/components/toast/toast-queue";
import { initials } from "../lib/format";
import { useTenant } from "../lib/hooks/use-tenant";

/** Top-left tenant switcher, fed by GET /me. */
export function TenantSwitcher() {
  const { tenant, tenants } = useTenant();
  const nav = useNavigate();
  const trigger = (
    <Tooltip content="Switch tenant" placement="right">
      <Button className="focus-ring flex w-full cursor-default items-center gap-2.5 rounded-lg border border-line bg-raised/70 p-2 text-left shadow-[var(--shadow-e1)] data-hovered:border-line-strong">
        <span className="flex size-7 items-center justify-center rounded-md bg-gradient-to-br from-hue-identity to-primary-strong text-[11px] font-semibold text-white">
          {initials(tenant?.name ?? "")}
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[13px] font-medium">{tenant?.name ?? "No tenant"}</span>
          <span className="block truncate text-[11px] text-fg-subtle">{tenant?.roles[0]?.replace("org_", "") ?? "—"}</span>
        </span>
        <ChevronsUpDown className="size-3.5 text-fg-subtle" />
      </Button>
    </Tooltip>
  );
  return (
    <Menu trigger={trigger} label="Tenants">
      {tenants.map((t) => (
        <MenuItem
          key={t.tenant_id}
          id={t.tenant_id}
          textValue={t.name}
          onAction={() => {
            nav(`/t/${t.tenant_id}`);
            toast.info(`Switched to ${t.name}`);
          }}
        >
          <span className="flex size-6 items-center justify-center rounded bg-overlay text-[10px] font-semibold">{initials(t.name)}</span>
          <span className="flex-1 truncate">{t.name}</span>
          {t.tenant_id === tenant?.tenant_id && <Check className="size-4 text-primary" />}
        </MenuItem>
      ))}
    </Menu>
  );
}
