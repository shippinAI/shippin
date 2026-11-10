# Team management UI (`/admin/team`)

Panel section **Team**, visible with `team.read`; editing needs `team.write` (Owner).

## List view
Columns: name, email, roles (chips), status (active / invited / deactivated), MFA (on/off,
type), last login, added by, added at. Filters by role and status. Data comes from Nomen
(user search scoped to the operator org + project authorizations) and the console audit log.
Last login: Nomen user "last login" / session data **[guess: verify the v2 API field]**.

## Actions
- **Invite by email**: email, display name, roles. Creates a human user in the operator org,
  sends Nomen's init email, adds the authorization. Status "invited" until first login.
- **Assign / revoke role**: per-role toggles with a required reason. Revoking ends sessions.
- **Deactivate / reactivate**: Nomen user deactivate; keeps history, blocks sign-in.
  Delete is not offered (audit continuity); deactivated users purge after policy **[guess]**.
- **Access review**: quarterly prompt listing every staff member + roles + last login; Owner or
  Security confirms or removes. Users idle > 90 days are flagged (NIST AC-2(3)).

## Guards (server-side, not just UI)
- **Last Owner**: cannot revoke `staff.owner` or deactivate a user if it leaves zero active Owners
  (409). Recommend >= 2 Owners; the page warns when only one.
- **No self-escalation**: you can't add roles to yourself; you may remove your own non-Owner roles.
- **SoD conflicts** from `catalog.md` are refused unless an Owner records a waiver reason.
- **MFA required**: staff without a second factor get no staff permissions (`/me` shows
  `mfa_required`). Enforce in Nomen with a login policy on the operator org (MFA, prefer
  passkeys/U2F) and re-check `amr` in the token server-side **[guess: claim availability]**.
- **Service account** for Team calls: a Nomen machine user with only `ORG_USER_MANAGER` +
  project grant management in the operator org, not IAM_OWNER.

## Audit
Every change writes to the append-only `audit_log` (owned by `shippin_audit_owner`):
`team.invite`, `team.role_add`, `team.role_remove`, `team.deactivate`, `team.reactivate`,
`team.waiver`, `team.review`, with actor, target, before/after roles, reason, IP. Fail-closed:
if the audit write fails, the change is not sent to Nomen.

## Two-person approval (optional, recommended)
For granting `staff.owner` or `staff.security` and for tenant delete: the change is stored as a
pending request; a different Owner must approve within 24 h **[guess]**; the requester can't approve.
With a single Owner this is impossible, so it activates only when >= 2 Owners exist.
