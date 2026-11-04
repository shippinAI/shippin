# System identity and human review

This is a migration and implementation plan. It does not describe a deployed
feature. The product is a self-hosted Shippin installation with one panel, one
system inventory and configurable human-in-the-loop behavior. Gold approvals
must not exist as a Shippin or AgentHub product feature, fixed policy, route,
identity or default. AgentHub owns the configurable review policy and review
records; the control plane owns the authoritative system inventory.

> **Status (2026-11-04):** the console and its one `web/` panel are in this
> repository. Gold signing, compiled approver defaults and the `HUB_OWNER_*`
> bypass have been removed from Shippin main. The hypervisor-local board adapter
> and `hv.Guard` have also been removed; every provider mutation now fails closed
> until the AgentHub path exists. The implementation gap is tracked in
> [configurable human review](approval.md). Shippin now persists hostname
> source/time on enrollment and exposes audited aliases, tags and explicit
> AgentHub tenant/machine mappings on the existing infrastructure screen for
> attached and unattached registered systems; this does not yet verify mappings
> with AgentHub or run its review workflow. The `internal/agenthub` client now
> backs an operator-configured endpoint and a read-only policy metadata check in
> the existing Platform settings API and UI. Its tests use a simulated REST
> server and do not establish live AgentHub compatibility. It is not connected
> to provider authorization, so provider mutations remain unavailable.
> Source ownership is defined by [source of truth](source-of-truth.md).

## Grounded starting point

- The active server registry owns the Shippin UUID. Migration
  `044_shippin_system_identity.sql` records hostname source/time at hostd
  enrollment, operator aliases/tags and explicit AgentHub tenant/machine IDs;
  the infrastructure UI reads and edits this profile. Operator metadata records
  actor/time and is audited. The draft compute-node API remains a separate
  contract and is not yet implemented as a replacement inventory.
- AgentHub `server/internal/machines/list.go` and migration
  `004_devagotchi_access.sql` define a separate machine with a text ID, label,
  bridge identity, host relationship and connection state. Its machine API and
  UI create that record separately. AgentHub's bridge already calls `os.Hostname`
  in diagnostic responses, but the reviewed machine model does not persist an
  observed hostname or associate it with the planned platform UUID.
- AgentHub `server/internal/changepolicy/` already stores versioned policy,
  tickets, evidence and reviews. A review binds to a ticket hash. Its current
  preset contains installation-specific rules and a named principal, and
  `server/internal/policyrules/eval.go` recognizes a Gold-specific runner.
- The currently reviewed REST decision response is boolean plus a policy hash,
  version, decision ID and matched rule IDs. No match defaults to allow. The
  `ticket` policy check proves only that a ticket exists; it does not bind the
  request's machine or command to that ticket. `passed` status alone is not a
  configured quorum. Shippin's adapter must verify exact ticket binding and the
  current policy's matched rule semantics; only AgentHub evaluates reviewer
  eligibility and configured quorum.
- Historical console `web/src/features/gold-sign/`, `internal/goldsign/` and
  `internal/api/goldsign_routes.go` implement separate Gold-specific screens,
  storage, signing and routes. They are not the AgentHub policy authority and
  are not evidence that a generic integrated workflow exists.

## Identity and provenance

Represent each real machine once in the control-plane inventory. If the control
plane is installed directly on a single VM, that VM and the control-plane host
are the same system: initialize one record and observe the VM's hostname from
that system. If the control plane runs in a container or manages another host,
do not silently substitute the container's or the control plane's hostname for
the remote machine. Report the actual observation source and leave unknown
values unknown until a trusted source reports them.

Use a stable Shippin system ID for references and authorization. Store the
observed hostname separately from operator-provided nickname, aliases and tags;
default the display to the hostname when no nickname is set. Hostnames can
change or collide, and aliases and tags are not proof of identity. Associate an
AgentHub bridge or provider observation with a system through an authenticated,
explicit enrollment/mapping, not a hostname-only match. Store source, observation
time and freshness alongside observations; distinguish user input from observed
facts. Hostname failure or stale data must not create a duplicate or claim a
system is online. Follow the distinction between identity and descriptive names
in [OpenTelemetry's host conventions](https://opentelemetry.io/docs/specs/semconv/resource/host/)
and [Kubernetes names and UIDs](https://kubernetes.io/docs/concepts/overview/working-with-objects/names/).

The draft inventory response now permits `provider: null` for an unspecified
provider; that is not proof that a provider is absent or that the system is ready.
The existing `name` remains for compatibility, while the panel prefers nickname,
then observed hostname, then name. Earlier consumers may reject a null provider;
coordinate their migration before a server emits it. The added response fields do
not create a system, discover a hostname, change a provider or save metadata.

Human-review requests and audit records must identify the same Shippin system ID
and include a snapshot of the relevant hostname, alias, provenance, action,
actor, time and policy version. A rename must not change what a prior decision
authorized. Policy scope and reviewers are user-configured; do not infer a
review requirement, named approver or special runner from a hostname or alias.

## Implementation order and proof

1. **Establish identity ownership.** Trace the complete control-plane inventory,
   AgentHub machine registration, bridge identity, grants and all callers. Define
   the canonical system record and an authenticated reference/mapping for existing
   AgentHub machine IDs. Update the existing API contract, storage and callers
   together; do not create independent inventory stores that can drift. Migrate
   existing IDs with reversible mappings and record ambiguous matches for human
   resolution. Preserve grants and history. Test one VM as one system, multiple
   providers, duplicate hostnames, re-enrollment, collisions and lost connectivity.
2. **Observe and manage metadata.** Collect the hostname from the machine where
   the observation actually runs, including source and timestamp. Provide UI and
   API editing for nickname, aliases and tags, with validation, authorization,
   optimistic concurrency and audit. Check restart persistence and behavior when
   hostname discovery fails or a hostname changes. Test a VM directly hosting the
   control plane and a separate remote host; do not assume container hostname is
   host hostname.
3. **Make human review policy data.** Reuse and adapt AgentHub's versioned
   policy, ticket and review capabilities. Replace Gold-specific runner parsing,
   seeded identities, geographic rules and filesystem assumptions with generic,
   operator-configured policy and explicitly configured execution integrations.
   A new installation must not inherit a named person's policy. Preserve existing
   policy versions and migrate active enforcement deliberately: removing a legacy
   default must not silently turn an existing denied action into an allowed one.
   Test absent/invalid policy, version conflicts, stale reviews, unauthorized
   reviewers, provider failures and restart recovery.
4. **Retire the legacy feature.** Inventory the console's routes, passkey and
   verifier callers, jobs, tests, configuration and data. Remove Gold-specific
   product navigation, API and code only when their effects and historical data
   have a reviewed replacement or archival path. Never reinterpret an old signed
   envelope as a new AgentHub review. Preserve audit history and revoke or migrate
   credentials using their actual owner. Compatibility paths, if needed during
   migration, must be temporary, documented and tested; they must not be a new
   product feature.
5. **Unify the UI and validate the path.** Place AgentHub policy configuration
   and human review under the AgentHub capability in the single panel; system detail and
   review provenance link to the same inventory record. Group files, API clients,
   tests and UI by capability. Verify with real navigation that each supported
   destination is reachable in three or four actions from the relevant starting
   screen, including keyboard and mobile navigation. Direct links, unavailable
   services, authorization failures and policy version changes must behave
   correctly. Do not create a Gold-specific destination or fixed host name.
6. **Run and ship.** Build from a clean checkout, start an isolated self-hosted
   installation, enroll a disposable VM and exercise configuration, identity,
   review, restart, upgrade, backup/restore and rollback. Verify behavior against
   real Incus and Proxmox hosts when those providers are implemented. Review all
   source changes and secrets before pushing, then merge the verified head after
   required checks pass. Until these checks pass, no deployment or merge is
   claimed and the old repositories and live policy data remain intact.
