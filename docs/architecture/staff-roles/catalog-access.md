# Customer catalog access: granular permissions, deny by default (the operator's ruling 2026-10-18 15:19 PDT)

Cloud-IAM style: one permission per thing a person can see or do. Only the Owner preset (and the
legacy IAM platform-admin gate, which is above it outside `enforce`) holds them. Every other
built-in role, including Support, Security / Compliance, Read-only Auditor and Billing, holds none by default.
Anyone else needs a direct grant: one permission, for one person, audited, revocable.

| Permission | Allows |
|---|---|
| `catalog.plans.read` | Plans, service catalog, plan x service matrix, tier defaults (no prices) |
| `catalog.prices.read` | The `price_cents` field (omitted from every response without it) |
| `catalog.plans.write` | Create, edit, delete plans, services and matrix cells |
| `config.defaults.write` | Change tier defaults and lock modes |
| `customers.read` | Customers, their service overrides and provision intents (no contacts, endpoints or notes) |
| `customers.contacts.read` | The `contacts` and `endpoints` fields |
| `customers.notes.read` | The `notes` field |
| `customers.write` | Create, edit, delete draft customers |
| `customers.services.write` | A customer's service overrides |
| `config.effective.read` | A customer's effective configuration |
| `provision.intent.create` | Record a provision intent (desired state for a later Gold ticket) |

`billing.read` and `billing.write` are reserved again (they gate nothing).

## Enforcement (the API, not only the UI)
- Every catalog route declares one of these permissions (`internal/catalog/http.go`); the operator gate
  answers 401 without a session and 403 without the permission, before handler code runs.
- Field-level: without `catalog.prices.read` / `customers.contacts.read` / `customers.notes.read` the
  server leaves the field out of the JSON. A write without the read permission keeps the stored value
  (an edit can't blank what the editor can't see) and a create that sets such a field gets 403.
- Roles: the granular permissions can't be put into any role but `staff.owner` (400).
- Direct grants (`staff_perm_grants`, migration 012) count only for a caller who already has a staff
  role and MFA, like every staff permission; a failed read of the grants fails closed.

## Grant / revoke (Team > Direct grants)
- `GET /v1/operator/team/permission-grants` (team.read): grants plus the grantable catalog.
- `POST /v1/operator/team/permission-grants` `{subject, permission, reason}` (team.write = Owner):
  grantable permissions only, never yourself, reason 5-300 chars, audited (`team.perm_grant`) in the
  same transaction as the write.
- `DELETE /v1/operator/team/permission-grants/{userId}/{permKey}?reason=...` (team.write), audited
  (`team.perm_revoke`). Both flush the staff cache, so the change applies on the next request.

## Tests
- `internal/api/testdata/staff-matrix/*.txt` (two files, each <= 150 lines): no preset but Owner lists
  a catalog route; `TestStaffMatrixDenyByDefault` checks every role x route.
- `staff_grants_test.go`: only Owner holds the 11 permissions; a grant opens exactly its routes; the
  grant API is Owner-only, audited, refuses self-grants and non-grantable keys; roles can't hold them.
- `internal/catalog/http_fields_db_test.go` (needs SHIPPIN_TEST_DATABASE_URL): prices, contacts and
  notes are left out and kept on update. `internal/db/perm_grants_db_test.go`: migration 012 on an
  existing database changes only `staff.owner`.
