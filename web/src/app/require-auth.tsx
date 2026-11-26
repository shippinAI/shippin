import { Navigate, Outlet } from "../lib/router/router";
import { useMe, usePublicSettings } from "../lib/hooks/use-api";
import { BrandMark } from "../template/brand-mark";

/** Gate: a signed-out visitor goes to /login, or to /setup while the installation has no owner (REL-03). */
export function RequireAuth() {
  const me = useMe();
  const pub = usePublicSettings();
  if (me.isPending || (me.isError && pub.isPending))
    return (
      <div className="flex h-full items-center justify-center">
        <div className="animate-pulse motion-reduce:animate-none">
          <BrandMark size={40} />
        </div>
      </div>
    );
  if (me.isError) return <Navigate to={pub.data?.setup_open ? "/setup" : "/login"} replace />;
  return <Outlet />;
}

export function LandingRedirect() {
  const me = useMe().data;
  if (sessionStorage.getItem("shippin.bridge.device")) return <Navigate to="/device" replace />;
  const t = me?.tenants[0];
  if (t) return <Navigate to={`/t/${t.tenant_id}/workspaces`} replace />;
  const caps = me?.capabilities ?? {};
  return (
    <Navigate
      to={
        caps["provider.panel"]
          ? "/admin"
          : caps["ops.view"]
            ? "/ops/servers"
            : caps["operator.view"]
              ? "/operator"
              : caps["infra.hosts"]
                ? "/operator/hosts"
                : caps["provider.panel"]
                  ? "/admin"
                  : "/welcome"
      }
      replace
    />
  );
}
