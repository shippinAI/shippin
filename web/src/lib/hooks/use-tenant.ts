import { useParams } from "../router/router";
import { useMe } from "./use-api";

/** Current tenant from the URL, falling back to the user's landing tenant. */
export function useTenant() {
  const { tenantId } = useParams();
  const me = useMe();
  const tenants = me.data?.tenants ?? [];
  const tenant = tenants.find((t) => t.tenant_id === tenantId) ?? tenants[0];
  return { tenant, tenants, me: me.data, isOperator: (me.data?.platform_roles.length ?? 0) > 0 };
}
