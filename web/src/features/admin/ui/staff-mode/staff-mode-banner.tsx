import { ShieldCheck } from "lucide-react";
import { Button } from "../../../../lib/design/components/button/button";
import { Navigate, useLocation } from "../../../../lib/router/router";
import { useExitStaffMode, useStaffMode } from "../../../../lib/hooks/use-staff-mode";

/** Shown while a staff-mode session is open: reason, expiry and an audited way back. When the
 *  server requires staff mode and none is open, it sends the user to the entry page. */
export function StaffModeBanner() {
  const q = useStaffMode();
  const exit = useExitStaffMode();
  const loc = useLocation();
  const s = q.data?.session;
  if (q.data?.required && !q.data.active && loc.pathname !== "/admin/staff-mode")
    return <Navigate to={`/admin/staff-mode?rd=${encodeURIComponent(loc.pathname)}`} replace />;
  if (!s) return null;
  return (
    <div role="status" className="relative z-10 flex items-center gap-2 border-b border-warning/30 bg-warning/10 px-8 py-2 text-sm text-warning">
      <ShieldCheck className="size-4 shrink-0" />
      <span className="truncate">Staff mode until {new Date(s.expires_at).toLocaleTimeString()} · {s.reason}</span>
      <Button size="sm" variant="secondary" className="ml-auto shrink-0" tooltip="Leave staff mode (audited) and go back to the tenant" isDisabled={exit.isPending} onPress={() => exit.mutate()}>
        Exit staff mode
      </Button>
    </div>
  );
}
