import type { MeIdentity, RuntimeConfig } from "../../shared/contracts";

export function ConnectionPanel({ config, tenantId, identity, onTenantChange }: {
  config: RuntimeConfig | null;
  tenantId: string;
  identity: MeIdentity | null;
  onTenantChange: (value: string) => void;
}) {
  return (
    <section className="panel">
      <h3>Connection</h3>
      <div className="field-grid">
        <label>API base URL<input value={config?.apiBaseUrl ?? ""} readOnly /></label>
        <label>
          Tenant ID (used for {"{tenantId}"})
          <input value={tenantId} onChange={(event) => onTenantChange(event.target.value)} placeholder="UUID tenant id" />
        </label>
        {identity?.tenants?.length ? (
          <label>
            Tenant from /me
            <select value={tenantId} onChange={(event) => onTenantChange(event.target.value)}>
              {identity.tenants.map((tenant) => (
                <option key={tenant.tenant_id} value={tenant.tenant_id}>
                  {tenant.name ? `${tenant.name} (${tenant.tenant_id})` : tenant.tenant_id}
                </option>
              ))}
            </select>
          </label>
        ) : null}
      </div>
    </section>
  );
}
