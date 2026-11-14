# Runner service panel integration

Status: development integration, not deployed or owner-accepted.

The existing panel owns runner navigation. The runner repository owns delegation,
scoped authorization, control, and storage. No runner source dependency is added to
the platform: a private TLS service is the boundary.

Server configuration:
- SHIPPIN_RUN_GATEWAY_ORIGIN: exact private HTTPS origin, no path or credentials.
- SHIPPIN_RUN_GATEWAY_CA: absolute regular PEM CA file for that service.

Both unset keeps runner routes explicitly unavailable (503); partial or invalid
configuration fails startup. Never disable certificate validation. Gateway private
signing keys and actual subject/tenant/project grants are configured only in the
runner service. No installation account or authorization is inferred by this work.

The platform proxies /v1/runner/projects and /v1/projects/ using only the existing
shippin_session cookie and allowlisted action headers. TLS trust is explicit,
redirects are refused, requests and responses are bounded, errors sanitized, and
upstream cookies and private headers stripped. The runner gateway rechecks live
platform identity and project authority before issuing scoped control requests.

The panel uses live authorized project discovery, filters by selected tenant, and
shows permission-aware reads. Configuration writes and rotation are unavailable
until their backend contracts and audited flows exist. There is no fixture account
picker and no fabricated healthy runner pool.

Qualification: focused Go tests cover real TLS upstream forwarding, credential
filtering, disabled configuration, duplicate sessions, and redirect rejection.
Runner-owned tests cover bridge to real control API and project revocation.
Neither is a substitute for the pending complete platform-session browser flow.
Remaining acceptance includes configured real grants, platform login through
project selection into actual control records, persisted state after recreation,
credential rotation, trace correlation and a verified public deployment.
