import { Suspense, useEffect } from "react";
import { createBrowserRouter, RouterProvider, useLocation } from "../../lib/router/router";
import { toRoutes } from "../../lib/router/tree";
import { routes } from "../../app/routes.admin.gen";
import { usePublicSettings } from "../../lib/hooks/use-api";
import { BrandMark } from "../brand-mark";

// The /admin route table (src/routes/admin) lives in this lazy chunk, so the customer initial JS never
// carries operator code (CONSOLE-IA budgets); the operator app (ops.html) mounts the same routes itself.
const adminRouter = createBrowserRouter(toRoutes(routes));

/** Console-side /admin: once the operator app owns its own origin (operator.origin_only), send
 *  the browser there; until then render the panel here. The API enforces the same rule. */
export function AdminOrigin() {
  const s = usePublicSettings();
  const loc = useLocation();
  const target = s.data?.operator_origin_only && s.data.operator_url ? s.data.operator_url + loc.pathname + loc.search : "";
  useEffect(() => {
    if (target) window.location.replace(target);
  }, [target]);
  if (s.isPending || target)
    return (
      <div className="flex h-full items-center justify-center">
        <div className="animate-pulse motion-reduce:animate-none"><BrandMark size={40} /></div>
      </div>
    );
  return (
    <Suspense fallback={null}>
      <RouterProvider router={adminRouter} />
    </Suspense>
  );
}
