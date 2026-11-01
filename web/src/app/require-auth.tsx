import { Navigate, Outlet } from "../lib/router/router";
import { useMe } from "../lib/hooks/use-api";
import { BrandMark } from "../template/brand-mark";

/** Gate: unauthenticated users go to /login. */
export function RequireAuth() {
  const me = useMe();
  if (me.isPending)
    return (
      <div className="flex h-full items-center justify-center">
        <div className="animate-pulse motion-reduce:animate-none"><BrandMark size={40} /></div>
      </div>
    );
  if (me.isError) return <Navigate to="/login" replace />;
  return <Outlet />;
}

export function LandingRedirect() {
  const me = useMe().data;
  if (sessionStorage.getItem("shippin.bridge.device")) return <Navigate to="/device" replace />;
  const t = me?.tenants[0];
  if (t) return <Navigate to={`/t/${t.tenant_id}`} replace />;
  const caps = me?.capabilities ?? {};
  return <Navigate to={caps["ops.view"] ? "/ops/servers" : caps["operator.view"] ? "/operator" : "/welcome"} replace />;
}
