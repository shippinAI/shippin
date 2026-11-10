import { Suspense } from "react";
import { Outlet } from "../../lib/router/router";
import { ApiError } from "../../lib/api/client";
import { useAdminAccess } from "../../lib/hooks/use-admin";
import { BrandMark } from "../brand-mark";
import { AccessDenied } from "../../features/admin/ui/access-denied";
import { BreakGlassBanner } from "../../features/admin/ui/access/break-glass-banner";
import { StaffModeBanner } from "../../features/admin/ui/staff-mode/staff-mode-banner";
import { StaffBanner } from "../../features/admin/ui/staff-banner";
import { AdminMobileNav } from "./admin-mobile-nav";
import { AdminSidebar } from "./admin-sidebar";

/** Provider panel shell. The API enforces access; this only renders the 403 nicely. */
export function AdminLayout() {
  const access = useAdminAccess();
  if (access.isPending)
    return (
      <div className="flex h-full items-center justify-center">
        <div className="animate-pulse motion-reduce:animate-none"><BrandMark size={40} /></div>
      </div>
    );
  if (access.isError) return <AccessDenied status={access.error instanceof ApiError ? access.error.status : 0} message={access.error.message} />;
  return (
    <div className="flex h-full flex-col md:flex-row">
      <AdminSidebar />
      <AdminMobileNav />
      <div className="relative flex min-w-0 flex-1 flex-col overflow-y-auto">
        <BreakGlassBanner />
        <StaffModeBanner />
        <StaffBanner />
        <main className="relative w-full max-w-page px-4 py-5 md:px-8 md:py-7">
          <Suspense fallback={null}>
            <Outlet />
          </Suspense>
        </main>
      </div>
    </div>
  );
}
