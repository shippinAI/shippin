# Configurable human review

## Ownership

AgentHub owns review policy, change tickets, evidence, reviews and decision
history. Shippin owns the stable system inventory, provider actions and an
authenticated adapter to AgentHub. A hostname, nickname, server alias or provider
name is never an identity mapping or a policy rule. Review requirements and
reviewer eligibility must come from the installation's AgentHub configuration;
Shippin must not seed a person, role, host or default action policy.

## Current Shippin behavior and gap

`internal/approval.Policy` remains a Shippin-local permission policy for the
optional agent task board. It is not an AgentHub adapter and it cannot authorize
hypervisor changes. Compiled fallback approvers and example role rules have been
removed. The `HUB_OWNER_SUB` / `HUB_OWNER_EMAIL` direct-owner bypass was also
removed.

The former provider board drivers were not AgentHub:

- `hub` wrote cards into this Shippin installation's `internal/hub` store.
- `http` spoke this console's `/v1/hub/approvals` contract.
- `off` disabled that board; it was not a human-review policy source.

The adapter and `hv.Guard` path have been removed. Generic operator and tenant
hypervisor mutation routes still return `503 agenthub_review_unavailable`
before a provider call, regardless of Shippin-local rules or task-board status.
Reads, server configuration and provider health checks remain available.

Deployment generations have a separate operator-initiated AgentHub review flow
in `internal/api/deployment_agenthub.go` and `internal/api/deployment_authorization.go`.
It creates or resumes a ticket bound to the generation action hash and configured
system mapping, attaches bounded redacted evidence, submits it, and reads ticket
status back. The operator authorization action rechecks the current policy and
ticket, requires every matched rule to explicitly require both approval and a
role-based review quorum, and persists safe decision provenance. The UI reports
that authorization was recorded for audit and that no provider operation ran.
The deployment worker does not call this authorization path, and all catalog
runtime execution remains unavailable.

The legacy `approval.hv_board` operator override has been removed. Old values are
ignored. The provider mutation gate does not consult a Shippin-local rule or
board, so a local card cannot substitute for AgentHub authorization.

Read-only source review of AgentHub commit `97172d9` on
`feat/devspace-bridge-contract` confirms authenticated
`/api/v1/policy/decide` and `/api/v1/change-tickets` routes for policy decisions,
ticket create/read/update, evidence, submission and hash-bound reviews. Bearer
tokens resolve ordinary enabled AgentHub principals. Ticket creation is controlled
by AgentHub's `change.ticket` data policy; reviews by `change.review`. This is
not a distinct OAuth service-principal protocol: a dedicated non-human principal
and an administrator-issued expiring token appear possible, but their exact
roles and tenant access must be verified in an isolated AgentHub instance.

The current decision contract returns a boolean `allow`, policy hash/version,
decision ID and matched rule IDs, but no typed `review_required` result. The
Shippin deployment adapter compensates for the no-match allow behavior by
requiring a nonempty matched-rule set and loading the current policy document;
every matched rule must exist, be enabled, have `effect=require`, and require
both human approval and a positive role-specific reviewer quorum. It reads
policy and ticket before and after the decision, verifies the action-bound ticket
fields and hash, and requires a passed ticket. Ticket status alone is never used
as authorization, and human-readable errors are not parsed. This path is covered
by the AgentHub client and API fixtures but has not been verified against a live
AgentHub installation or a least-privilege production principal. Commit
`537f304` adds a lease-fenced store method for future worker decision provenance;
the worker does not call it until an executable runtime exists. The AgentHub
checkout remains dirty and untouched.
Do not describe the local task board as AgentHub or as the source of policy.

## Target request flow

1. Shippin resolves the authenticated operator to one stable Shippin system ID
   and an explicit, configured AgentHub tenant/machine mapping.
2. Shippin asks the configured AgentHub policy for the action and resource
   context. It records the policy version/hash and provenance returned by
   AgentHub.
3. If review is required, Shippin creates or retrieves an idempotent AgentHub
   change ticket with the exact action, system ID, provider resource ID,
   hostname observation, desired diff, evidence and rollback information.
4. AgentHub's authorized reviewer records the decision against the ticket hash.
   Shippin re-reads and validates the current ticket, policy version, reviewer,
   decision and system mapping before executing.
5. **Not implemented:** after the operator review flow records authorization,
   the worker must obtain a fresh lease-fenced AgentHub decision for the current
   action before dispatch. It then executes only through the configured provider
   adapter and writes provider read-back, health and provenance to Shippin's
   resource audit. A decision never changes the stable system ID.

AgentHub unavailability, missing/ambiguous mapping, stale policy/ticket hashes,
missing evidence or a denied decision blocks the gated action. Read-only
inventory can remain available if its own authorization permits it. Historical
review envelopes remain historical records and are never converted into
AgentHub decisions.

## Implementation requirements

- Configure AgentHub URL, authentication credential reference, tenant mapping,
  timeout and supported capabilities per installation. Never commit token
  values or tenant IDs.
- Validate the current policy/ticket API and verify an installation-configured,
  least-privilege AgentHub principal before connecting provider writes. Keep the
  adapter's explicit matched-rule and ticket-hash checks; do not treat the boolean
  response as sufficient. Do not copy AgentHub policy code or create a second
  policy store in Shippin.
- Keep one system ID across Incus, Proxmox, host enrollment, health, resource
  display, action tickets and audit. Store observed hostname with source and
  timestamp; nickname/aliases/tags remain operator metadata.
- Test no policy, invalid policy, policy version changes, stale tickets, wrong
  tenant/system, reviewer denial, self-review, unauthorized reviewer, expired
  credentials, AgentHub timeout, retry/replay, provider failure and restart.
- Keep the local Shippin task-board policy scoped to its own task workflow. The
  operator deployment-review adapter exists; worker-side use stays gated on a
  verified AgentHub principal, lease-fenced provenance and a complete provider
  executor with negative-path and recovery evidence.
  Preserve existing task/audit records; do not reset dirty AgentHub worktrees.

