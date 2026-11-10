# Test plan (deny by default)

## Unit (Go, table-driven, extends `authz_test.go`)
- **Route coverage**: every `operatorRoutes()` entry has a permission; test fails on an unmapped route.
- **Matrix test**: for each role x each route, expect 200/2xx or 403 exactly per `matrix.md`.
  Generated from the same table plus an independent hand-written expectation file, so a table
  typo can't make both sides agree.
- **No roles**: authenticated user with zero staff roles → 403 on every route, `provider.panel` false.
- **Wrong org**: staff role claim under a customer orgId → ignored, 403.
- **Unknown role key** (`staff.foo`) → no permissions.
- **MFA missing** → 403 even with Owner.
- **Legacy IAM_OWNER**: per mode legacy/shadow/enforce/retired, expected result and audit event.
- **Config override** can remove, never add, permissions.
- **/me parity**: capabilities equal the server permission set for every role.

## Guards
- Revoke last Owner → 409; deactivate last Owner → 409; with two Owners → 200.
- Self role add → 403. SoD conflict without waiver → 409; with waiver → 200 + `team.waiver`.
- Two-person: requester approve → 403; second Owner approve → applied; expiry → rejected.
- Audit fail-closed: audit insert error → change not sent to Nomen (fake IdP asserts no call).

## Consent and break-glass
- Support: request grant 200, break-glass 403, approve 403.
- Ops: break-glass request → pending; self-approve 403; Security approve → active; IDE 200.
- Billing/Auditor: request grant 403, IDE 403.
- Legal hold by non-Security → 403. Withheld > 30 days → 400.
- Owner without grant → IDE 403 (no implicit access).

## Live (throwaway identities)
One Nomen test user per role in the operator org; script hits each route and diffs against the
matrix. Never on the operator's dev tenant. Playwright: Team page invite/revoke/last-Owner guard screenshots.
