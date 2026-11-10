import { ShieldCheck } from "lucide-react";
import { Link } from "react-aria-components";
import { useMe, usePublicSettings } from "../lib/hooks/use-api";
import { useTenant } from "../lib/hooks/use-tenant";

/** Top-bar switch into the provider panel. Rendered only when GET /me grants provider.panel
 *  (IAM_OWNER, decided server-side); /admin and its API stay enforced by the server. */
export function AdminPanelLink() {
  const allowed = useMe().data?.capabilities["provider.panel"] === true;
  const s = usePublicSettings().data;
  const from = useTenant().tenant?.tenant_id ?? "";
  if (!allowed) return null;
  return (
    <Link
      href={`${s?.operator_origin_only && s.operator_url ? s.operator_url : ""}/admin/staff-mode?from=${encodeURIComponent(from)}`}
      aria-label="Admin panel"
      className="focus-ring inline-flex h-8 items-center gap-1.5 rounded-lg border border-warning/35 bg-warning/10 px-3 text-sm font-medium text-warning transition-colors data-hovered:border-warning/60 data-hovered:bg-warning/20"
    >
      <ShieldCheck className="size-3.5" strokeWidth={2} />
      Admin panel
    </Link>
  );
}
