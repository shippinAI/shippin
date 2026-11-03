# Nomen mapping and server-side resolution

## Where roles live
- **Project roles** in project `Shippin Console` (id ) on id.shippin.cloud:
  `staff.owner`, `staff.platform_admin`, `staff.ops`, `staff.support`, `staff.billing`,
  `staff.security`, `staff.auditor`. Group label `staff` (Nomen role "group" field).
- **Authorizations (user grants)** assign roles to a staff user, granted only in the Shippin
  operator org **[guess: name of the org that owns the project]**. Staff roles are never part of a
  project grant to customer orgs, so a customer org admin can't hand them out.
- Project settings: **Assert roles on authentication** on, **Check authorization on authentication**
  off (customers must still sign in). Tenant org roles keep working via `roles.org` as today.

## Token claim
Nomen puts project roles in `urn:nomen:iam:org:project:{projectId}:roles` (and the
generic `urn:nomen:iam:org:project:roles`) as `{role: {orgId: domain}}`.
The API accepts a staff role only if the orgId in that map is the operator org id from config
(`staff.org_id`); a role granted under any other org is ignored and logged.
Bearer PATs (machine users) do not get staff roles by default **[guess: CI may need `staff.ops`]**.

## Resolution in the console (server-side, per request)
1. `callerFor` reads the claim (or userinfo/introspection, same as `opRole` today), caches it with
   the existing caller cache TTL.
2. `staffRoles []string` → `perms map[string]bool` from a static Go table (`perm_roles.go`), with an
   optional config override that may only **remove** permissions, never add.
3. Each operator route is registered as `requirePerm("tenants.delete", h)`; unknown routes fail
   at startup (a test asserts every route in `operatorRoutes()` has a permission).
4. `/me` returns `staff_roles` and `capabilities` derived from the same table, so UI and API agree.
5. Every 403 from `requirePerm` is audited as `authz.denied` (rate-limited) for access reviews.
6. Revocation: role changes take effect at the next token refresh; for Owner/Security removals the
   team API also ends the user's sessions in Nomen so it is immediate.

## IAM_OWNER as break-glass super-owner only
- IAM_OWNER keeps Nomen instance control; it is needed to repair Nomen itself.
- In the console it grants **no** panel permissions by itself after cutover. Instead it unlocks a
  separate `superowner` path: fresh login (reauth 10 min), a justification, and a red
  `superowner.activate` audit entry, then 1 h of the full Owner permission set.
- Credentials: 1-2 named humans, hardware-key MFA, plus one sealed recovery account
  **[guess]**. Not used day to day; any use triggers a notice to all Owners and Security.
- This keeps the lockout escape (if all `staff.owner` grants are lost) without a standing god role.

## Config sketch (illustrative, not code)
```
staff:
  project_id: ""
  org_id: "<operator org id>"
  superowner_role: IAM_OWNER
  two_person: [staff.owner, staff.security]
```
