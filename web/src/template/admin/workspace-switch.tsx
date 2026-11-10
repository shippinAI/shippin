import { ArrowLeft, Check, ChevronDown } from "lucide-react";
import { Button, Link } from "react-aria-components";
import { useNavigate } from "../../lib/router/router";
import { Menu, MenuItem } from "../../lib/design/components/menu/menu";
import { initials } from "../../lib/format";
import { usePublicSettings } from "../../lib/hooks/use-api";
import { useTenant } from "../../lib/hooks/use-tenant";

/** Provider panel → tenant console: "Back to my workspace" plus a switch between the admin's own
 *  tenants (from GET /me; tenants seen only through an access grant are excluded). */
export function WorkspaceSwitch() {
  const { tenants } = useTenant();
  const own = tenants.filter((t) => !t.access);
  const nav = useNavigate();
  const s = usePublicSettings().data;
  // On the operator origin the tenant console is another origin: leave the app instead of routing.
  const base = s?.app === "operator" ? (s.console_url ?? "") : "";
  const go = (path: string) => (base ? window.location.assign(base + path) : nav(path));
  const home = base + (own[0] ? `/t/${own[0].tenant_id}/workspaces` : "/");
  return (
    <div className="mx-3 mb-2 flex items-stretch overflow-hidden rounded-lg border border-line bg-raised/60 shadow-e1">
      <Link
        href={home}
        className="focus-ring flex min-w-0 flex-1 items-center gap-2 px-2.5 py-2 text-sm font-medium text-fg data-hovered:bg-overlay"
      >
        <ArrowLeft className="size-3.5 shrink-0" />
        <span className="truncate">Back to my workspace</span>
      </Link>
      {own.length > 1 && (
        <Menu
          label="Switch tenant"
          trigger={
            <Button aria-label="Switch tenant" className="focus-ring flex items-center border-l border-line px-2 text-fg-subtle data-hovered:bg-overlay data-hovered:text-fg">
              <ChevronDown className="size-3.5" />
            </Button>
          }
        >
          {own.map((t, i) => (
            <MenuItem key={t.tenant_id} id={t.tenant_id} textValue={t.name} onAction={() => go(`/t/${t.tenant_id}/workspaces`)}>
              <span className="flex size-6 items-center justify-center rounded-sm bg-overlay text-2xs font-semibold">{initials(t.name)}</span>
              <span className="flex-1 truncate">{t.name}</span>
              {i === 0 && <Check className="size-4 text-primary" />}
            </MenuItem>
          ))}
        </Menu>
      )}
    </div>
  );
}
