# Configurable human review

## Ownership

AgentHub owns review policy, change tickets, evidence, reviews and decision
history. Shippin owns the stable system inventory, provider actions and an
authenticated adapter to AgentHub. A hostname, nickname, server alias or provider
name is never an identity mapping or a policy rule. Review requirements and
reviewer eligibility must come from the installation's AgentHub configuration;
Shippin must not seed a person, role, host or default action policy.

## Current Shippin behavior and gap

The current `internal/approval.Policy` is still a local policy engine, not the
AgentHub adapter. The compiled fallback approvers and example role rules have
been removed. An empty policy denies gated actions; the example config disables
the local board. The `HUB_OWNER_SUB` / `HUB_OWNER_EMAIL` direct-owner bypass was
also removed; the local task board now requires an explicit policy. This is a
fail-closed transition, not an AgentHub integration.

The current board drivers are also not AgentHub:

- `hub` writes cards into this Shippin installation's `internal/hub` store.
- `http` speaks this console's `/v1/hub/approvals` contract.
- `off` disables the board; it is not a human-review policy source.

The existing adapter has no proven compatibility with AgentHub's
`/api/v1/policy/*` and `/api/v1/change-tickets/*` API, tenant model, authenticated
principal, ticket hash, evidence or review version. No integrated AgentHub
review flow exists yet. Do not describe the local task board as AgentHub or as
the source of policy.

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
5. Shippin executes only through the configured provider adapter, then writes
   outcome and provenance back to the ticket/evidence flow and its own resource
   audit. A decision never changes the stable system ID.

AgentHub unavailability, missing/ambiguous mapping, stale policy/ticket hashes,
missing evidence or a denied decision blocks the gated action. Read-only
inventory can remain available if its own authorization permits it. Historical
review envelopes remain historical records and are never converted into
AgentHub decisions.

## Implementation requirements

- Configure AgentHub URL, authentication credential reference, tenant mapping,
  timeout and supported capabilities per installation. Never commit token
  values or tenant IDs.
- Discover and validate supported policy/ticket APIs from current AgentHub source
  and published contract. Do not copy AgentHub policy code or create a second
  policy store in Shippin.
- Keep one system ID across Incus, Proxmox, host enrollment, health, resource
  display, action tickets and audit. Store observed hostname with source and
  timestamp; nickname/aliases/tags remain operator metadata.
- Test no policy, invalid policy, policy version changes, stale tickets, wrong
  tenant/system, reviewer denial, self-review, unauthorized reviewer, expired
  credentials, AgentHub timeout, retry/replay, provider failure and restart.
- Replace the remaining local Shippin policy and board decision path with the
  AgentHub adapter only after its service-principal authentication, tenant/system
  mapping, idempotent ticket lifecycle and failure behavior are implemented.
  Preserve existing task/audit records; do not reset dirty AgentHub worktrees.

Historical Gold signing settings and routes were removed from Shippin main.
`GOLDSIGN_DIR` and `GOLDSIGN_FACTS_DIR` are only detected to emit a migration
notice; Shippin does not read those records. Keep any historical records under
their existing owner until separately archived or verified.
