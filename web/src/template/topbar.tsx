import { BookOpen, Command, RefreshCw } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { IconButton } from "../lib/design/components/button/icon-button";
import { toast } from "../lib/design/components/toast/toast-queue";
import { usePublicSettings } from "../lib/hooks/use-api";
import { lazy, Suspense } from "react";
import { AdminPanelLink } from "./admin-panel-link";
import { MobileNav } from "./mobile-nav";
import { useMe as useCurrentUser } from "../lib/hooks/use-api";
import { UpdateStatus } from "../features/updates/update-status";

// Loaded after first paint so the header stays inside the initial JS budget.
const OrgSwitcher = lazy(() => import("../features/orgs/ui/org-switcher").then((m) => ({ default: m.OrgSwitcher })));

/** Aero-glass top bar (one of the few glass surfaces allowed by 02d). */
export function Topbar({ crumb }: { crumb: string }) {
  const caps = useCurrentUser().data?.capabilities;
  const qc = useQueryClient();
  const s = usePublicSettings().data;
  return (
    <div className="glass sticky top-0 z-30 flex h-14 items-center gap-2 border-b border-line px-4 sm:gap-3 sm:px-8">
      <MobileNav />
      <div className="min-w-0 flex-1 truncate text-[13px] text-fg-subtle">
        <span className="hidden sm:inline">{s?.domain}</span>
        <span className="mx-2 hidden sm:inline">/</span>
        <span className="text-fg">{crumb}</span>
      </div>
      <div className="hidden items-center gap-1.5 rounded-md border border-line bg-canvas/50 px-2 py-1 text-xs text-fg-subtle md:flex">
        <Command className="size-3" /> K <span className="ml-1">Command palette · v0.2</span>
      </div>
      {caps?.["operator.view"] && <UpdateStatus />}
      <Suspense fallback={null}><OrgSwitcher /></Suspense>
      <AdminPanelLink />
      <IconButton
        icon={RefreshCw}
        label="Refresh data"
        onPress={async () => {
          await qc.invalidateQueries();
          toast.success("Data refreshed");
        }}
      />
      <IconButton icon={BookOpen} label="API reference (openapi v0.1)" onPress={() => toast.info("API docs", "Published with the control-plane API in v0.2.")} />
    </div>
  );
}
