# Staff roles and team management (design only)

Status: proposal, 2026-10-16. No code, deploys or Nomen changes yet.
Answers the operator: "do I have an admin panel to properly manage the Shippin team?" Today: **no**.
`/admin` and every `/v1/operator/*` route sit behind `platformAdmin` (`internal/api/operator.go`),
which passes only when `opRole()` maps the caller's Nomen instance role (IAM_OWNER via `roles.iam`)
to `operator.role`. All or nothing; no team page, no per-duty permissions.

## Build steps (what each does, what it touches)
1. **Permission catalog** - Go table of permission strings + role→permission map.
   Touches: new `internal/api/perm_*.go`, config `roles.staff`. No DB.
2. **Nomen project roles** - create role keys in project `Shippin Console` (),
   enable "assert roles on authentication". Touches: Nomen only (manual, approved separately).
3. **Resolver** - read `urn:nomen:iam:org:project::roles` claim, map to
   permissions in `caller`. Touches: `caller.go`, `roles.go`, `me.go` capabilities.
4. **Per-route gates** - replace the single `platformAdmin` wrap with `requirePerm(p, h)` per route.
   Touches: `operator.go`, `authz_test.go`.
5. **Team API + UI** - `/v1/operator/team/*` (invite, assign, revoke, deactivate) calling Nomen
   management API with a scoped service user. Touches: new `operator_team*.go`, `web/src/features/team/`.
6. **Approvals (optional)** - `staff_role_requests` table for two-person changes. Touches: migrations.
7. **Migration + cutover** - shadow mode, then enforce; IAM_OWNER becomes break-glass super-owner.
   Touches: config, `deploy/tests/`.

## Files
- `catalog.md` - roles and duties
- `matrix.md` - permission matrix over panel sections
- `nomen-mapping.md` - Nomen roles, grants, server-side resolution, IAM_OWNER
- `team-ui.md` - team management page and guards
- `access-interplay.md` - consent access and break-glass by role
- `migration.md` - rollout from the IAM_OWNER gate
- `test-plan.md` - deny-by-default tests per role
- `questions.md` - decisions only the operator can make
- `sources.md` - references

Marking: **[guess]** = assumption not verified against code or the operator.
