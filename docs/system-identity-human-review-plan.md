# System identity and human review

This is a migration and implementation plan. It does not describe a deployed
feature. The product is a self-hosted Shippin installation with one panel, one
system inventory and configurable human-in-the-loop behavior. Gold approvals
must not exist as a Shippin or AgentHub product feature, fixed policy, route,
identity or default. AgentHub owns the configurable review policy and review
records; the control plane owns the authoritative system inventory.

## Grounded starting point

- The draft platform compute API (`api/openapi/paths/compute-nodes.yaml` and its
  schema) uses a UUID, name and Incus/Proxmox provider. Enrollment and the
  control plane are planned; `panel/src/features/infrastructure/` only reads a
  planned API. The agent currently supplies read-only provider diagnostics.
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
5. **Unify the UI and validate the path.** Place policy configuration and human
   review under the AgentHub capability in the single panel; system detail and
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
