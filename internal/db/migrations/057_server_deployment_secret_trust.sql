-- Tenant deployment secrets may be projected into provider-readable workload
-- configuration only when an operator explicitly accepts that provider's
-- administrators can read them. Existing and newly added targets deny this by default.
ALTER TABLE servers
  ADD COLUMN IF NOT EXISTS hv_allow_tenant_deployment_secrets boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN servers.hv_allow_tenant_deployment_secrets IS
  'Operator acknowledgement that hypervisor administrators can read tenant deployment secrets projected into workload configuration.';
