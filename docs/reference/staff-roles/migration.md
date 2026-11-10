# Migration from the IAM_OWNER-only gate

Today: `platformAdmin` → `isPlatformAdmin` → `c.op == operator.role`, where `c.op` comes from
`roles.iam` (IAM_OWNER). `/me` `provider.panel` and `tenants.create` use the same check.

## Phases
1. **Catalog + Nomen roles** (no behavior change): add project roles; grant `staff.owner` to the
   current IAM_OWNER humans (the operator **[guess]**). Verify the claim shows up in tokens.
2. **Shadow mode**: compute permissions alongside the old gate; log `authz.shadow_mismatch`
   when they disagree. Old gate still decides. Run >= 1 week or until zero mismatches.
3. **Enforce**: switch routes to `requirePerm`; IAM_OWNER still maps to the Owner set as a
   compatibility fallback, logged as `authz.legacy_iam_owner` on every use.
4. **Team page live**: invite real staff with narrow roles.
5. **Retire fallback**: IAM_OWNER only via the superowner path (reauth + red audit). Remove
   `operator.role` and `roles.iam` panel use; keep `roles.org` for tenants.
6. **Two-person** on once there are >= 2 Owners.

Rollback: each phase is a config flag (`staff.mode: legacy|shadow|enforce`) **[guess]**; legacy
restores today's behavior exactly.

## Risks
- Lockout if the claim isn't asserted: phase 1 verifies with a throwaway identity first, and
  IAM_OWNER fallback stays until phase 5.
- Stale tokens after revocation: session termination on high-risk removals.
- Audit DB role: A2 (triggers droppable by the console role) should be closed (owner
  `shippin_audit_owner`) before relying on team audit.
