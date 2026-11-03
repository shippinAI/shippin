# cp-p0b-orgs: Operations vs customer orgs (CONTROL-PLANE-DEVSPACE-PLAN.md §3a)
Box only; nothing deployed. Based on cp-p1-servers.

- **Migration 032_orgs.sql** (rollback `internal/db/rollback/032_orgs.down.sql`): `orgs` (`type` operations|customer,
  a unique partial index allows exactly one Operations org), `folders` (org → nested folders), `staff_assignments`
  (ops_admin/ops_oncall/ops_support/ops_auditor, optional expiry, revocable) and `support_grants` (staff → one customer
  org/project, reason ≥ 10 chars, `expires_at` required, capped by config, revocable).
  Named `staff_assignments`, not `staff_roles`: migration 007 already has `staff_roles` (the staff.* role catalog).
- **Config `orgs`** (off by default): `operations_slug`, `operations_name`, `operations_nomen_org_id`,
  `bootstrap_admin_sub` (a Nomen subject, made ops_admin on first use; not a secret), `support_grant_max_seconds`.
- **API**: `GET /v1/orgs` (switcher: Operations if staff + one customer org per tenant membership, created on first
  sight); `GET /v1/ops/servers` (any staff role, no tenant or devspace needed; tenant roles get 403);
  `GET|POST /v1/ops/support-grants`, `POST /v1/ops/support-grants/{id}/revoke` (ops_admin; audit row written first,
  no audit → no grant). `/v1/me` capability `ops.view` sends staff without a devspace to `/ops/servers`.
- **Web**: header org switcher (React Aria menu, lazy-loaded; Operations shown amber with a "staff" tag), page
  `/ops/servers` (lazy). Initial JS 89,888 B gz of 90,000 (base 89,744).
- **Tests**: `go test -p 1 ./internal/...` with `SHIPPIN_TEST_DATABASE_URL` (disposable Postgres); Playwright
  `deploy/screenshots/shots-orgs.mjs` → `docs/screens/cp-p0b-orgs/`.
- **Not done / gaps**: support grants are stored and audited but not yet *enforced* on tenant routes (staff still see
  no tenant data, which is the safe direction); OpenFGA tuples for org/folder/project not written; the existing
  Zitadel staff catalog (staff_roles, team pages) is not reconciled with ops roles; the sidebar still shows the
  customer devspace nav while in Operations; this branch has no phone shell layout (pre-existing), so the switcher
  is off-screen at 390 px.
