# Staff role catalog

Principles: least privilege (NIST AC-6), separation of duties (AC-5), account lifecycle (AC-2).
Roles are additive; a person may hold several, but certain pairs are blocked (see bottom).
Role keys are Nomen project role keys in `Shippin Console` (prefix `staff.`).

| Key | Name | One-line duty |
|---|---|---|
| `staff.owner` | Owner | Runs the company account: manages the team and every setting; min 2 holders recommended. |
| `staff.platform_admin` | Platform Admin | Configures the platform: templates, mail, DNS, edge, tenant lifecycle. No team or billing changes. |
| `staff.ops` | SRE / Ops | Keeps hosts, VMs and workspaces running: restart, reprovision, view host/edge. No deletes. |
| `staff.support` | Support | Helps customers: reads tenant metadata, requests consent access; no tenant data without a grant. |
| `staff.billing` | Billing | Manages plans, invoices, credits; reads tenant names/status only. |
| `staff.security` | Security / Compliance | Reads full audit, approves break-glass, sets legal hold, reviews team. No ops actions. |
| `staff.auditor` | Read-only Auditor | Reads audit and configuration, read-only everywhere; time-boxed for external auditors. |

IAM_OWNER (Nomen instance) is **not** a staff role; it is the break-glass super-owner (see `nomen-mapping.md`).

## Duties in detail
**Owner** - invite/remove staff, assign any role, approve high-risk role changes, tenant delete,
billing settings. Can request break-glass but cannot self-approve it.

**Platform Admin** - templates CRUD, mail and DNS config, edge config, tenant create/suspend/resume,
VM enable. Tenant delete only with Owner co-approval **[guess: the operator to confirm]**.

**SRE / Ops** - host, edge, VM and workspace actions (start/stop/restart/retry provision), reads
tenants and workspaces metadata. May request consent access and break-glass (incident use).

**Support** - tenant and workspace list (metadata only), user lookup in Users & sign-in (read),
request consent access scoped to tenant or workspace. No suspend, no delete, no break-glass.

**Billing** - Billing section full; tenant list read (name, status, plan). Nothing else.

**Security / Compliance** - Access & audit full read incl. break-glass red entries and withheld
notices; approve/deny break-glass; apply legal hold (`withhold_notice`); read Team; run access reviews.

**Read-only Auditor** - read on Tenants, Host, Edge, Templates, Mail, DNS, Access & audit, Team.
No Users & sign-in PII beyond counts **[guess]**, no actions, auto-expiry on the role grant.

## Separation-of-duties conflicts (blocked combinations)
- `staff.security` + `staff.ops` / `staff.platform_admin`: approver must not be a likely requester.
- `staff.billing` + `staff.owner` is allowed (small team) but flagged in the access review.
- `staff.auditor` + any write role: auditor must be independent.
With a tiny team these may need an explicit, audited waiver (see `questions.md`).
