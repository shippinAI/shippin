# Shippin capability absorption ledger

Status: active execution ledger. `shippin-platform` is the only writable product
checkout. This document records source evidence and platform disposition; it does
not claim that a design or catalog row is a running deployment.

## Source checkout audit

| Source | Audited revision and state | What it contains | Disposition and guard |
|---|---|---|---|
| `shippin-platform` | `main` `7afa89f390bea41778cd07ad0a10f654e36093b1`; CI run `41852357913` green | Go control plane, host agent, Incus/Proxmox drivers, one React panel, configured network inventory, catalog source metadata; local approver defaults removed; server connection configuration remains role-gated and audited | Canonical implementation owner. Continue in existing `internal/`, migrations, API, provider and `web/` feature owners. |
| `shippin-console` | local `main` `93dd341`; `origin/main` `e35ab81` (164 commits ahead of the local checkout); local untracked `deploy/bastion/` contains Python cache artifacts | `e35ab81` is the historical console snapshot documented as imported into this repository. Its tracked bastion source includes verified-bundle launch, Incus-specific validation, atomic recovery, nftables/systemd changes and lifecycle tests. The change module identifies itself as Gold-policy-only and requires an approved inline launcher. | No merge from this checkout. Preserve the local cache artifacts and source branch. Re-author safe host operations under Shippin Prox only after adapting to the Shippin/AgentHub authorization, provider and rollback contracts; do not carry Gold naming or approval assumptions. |
| `shippin-cloud` | `main` `49475d3b`; clean; 177 tracked files | Historical product plans, OpenAPI draft and a 29-file React/Vite workbench. Its own README names `shippin-platform` as the current public product. Plans describe Incus tenancy, future Proxmox, service catalog/deployments, Nomen/Vaultix tracks and release workflow; several passages retain old site facts. | Workbench is superseded by `web/`; API/architecture notes are evidence to reconcile, not a second panel or current runtime. Configuration and deployment details must be revalidated. |
| `shippin-mesh` | `main` `26cce35b`; dirty `go.work.sum`; 790 tracked files; no root README or root license observed | 208 files under `warden/`, 90 Go and 104 Python files overall, 177 docs, 157 scripts; includes internal Proxmox tooling, Warden panel, agent/MCP code, templates, registries, infra and AI/context modules. Root `AGENTS.md` explicitly describes installation-specific internal infrastructure and says customer contracts must be normalized behind Shippin IDs. | Read-only source. No code/artifact is imported from it. Its five formerly unavailable named docs are present locally; stale 404 statements must be removed. Root license/provenance and each dependency/asset license remain unresolved. Site-specific URLs, IDs, addresses, modules and shell automation are not product defaults. |
| `third_party/warden-ct102` | Reference import already inside platform; 188 tracked files | Older Go/HTML operator panel and 216 matrix rows, including 28 screenshot rows that are not present in the current tree | Never served and never a second panel. Existing file-by-file matrix remains the detailed ledger; revalidate every `to-author` row and stale status before archive readiness. |
| `agent-hub` | branch `feat/devspace-bridge-contract`, HEAD `97172d9a`; extensive modified and untracked user work | Tenant-scoped policy documents, versioned changes, evidence and reviews in `server/internal/changepolicy`; authenticated HTTP routes under `/api/v1/policy/*` and `/api/v1/change-tickets/*`; machine IDs and an existing bridge identity model | Read-only source; do not edit/reset. AgentHub owns policy and review records. Shippin's board adapter is not proof of AgentHub compatibility. Integrate only through an authenticated, explicitly configured system/machine mapping and tested policy/review calls. |

The public `shippin-console` remote tip is the imported `e35ab81` described in
`docs/merge-from-shippin-console.md`; the 164-commit difference is between that
remote tip and the older local checkout. It is not 164 unmerged commits to Shippin
main. Bastion source is tracked at the remote tip; local untracked files are only
cache artifacts. The source tree and local files remain untouched.

## Capability disposition

| Capability | Canonical owner | Current evidence/status | Remaining implementation and acceptance |
|---|---|---|---|
| One panel, navigation and design system | `web/`, `cmd/console` | Implemented as one React panel; historical `panel/` workbench moved to API explorer. | Keep feature files, clients, schemas and tests colocated. Verify role-aware routes and each destination in at most four actions. No sibling panel or Warden route. |
| Sign-in and operator identity | `internal/auth`, Nomen integration | Nomen is the identity source; Gold signing was removed from main. | Verify setup, role mapping, hostname observation and restart persistence; do not reuse a second identity/PAM store. |
| AgentHub human review | AgentHub owns policy; Shippin owns system identity and adapter | AgentHub API is tenant-scoped and versioned; existing Shippin hub board is a separate contract. Shippin compiled approver defaults and `HUB_OWNER_*` bypass have now been removed. No integrated AgentHub workflow yet; example config denies gated actions. | Implement authenticated service-principal calls, tenant/system mapping, policy lookup, ticket/evidence/review round trip, fail-closed behavior, provenance and UI navigation. Keep policy and reviewers installation-configurable in AgentHub. |
| Incus/Proxmox server inventory and lifecycle | `internal/servers`, `internal/hv/{incus,proxmox}`, `internal/api`, infrastructure UI | Both drivers, add-host and lifecycle flows exist. Proxmox placement and resource telemetry have prior implementation; live coverage is limited. | Confirm create/edit/test for both provider kinds, guest types, resource limits, retries, audit and provider read-back. Keep provider-specific lifecycle code clear and distinct. Test supported Incus VM/container and Proxmox VM/CT paths on disposable hosts. |
| GitHub/custom Git/OCI catalog source | `internal/catalog`, migration `043`, catalog UI | Pinned source metadata persists and validates shape. Current CI passed; catalog DB CRUD flow was not locally run. | Run disposable-DB create/read/update; define immutable fetched content digest, provenance and bounded build contract. No fetch/build/deploy exists yet. |
| Deployment definitions and reconciliation | Existing catalog/API/jobs/hostd/provider owners after contract review | Provision intents currently record pending desired state; no general source executor or observed deployment reconciler. | Add prefixed desired/observed records, idempotent bounded worker, safe source fetch/build, configured secret resolution, provider adapters, health/read-back, events, retry/restart/delete. Prove with fake and disposable provider. |
| Managed PostgreSQL, Dragonfly/Redis and ClickHouse | Managed-service bounded context, hostd/provider adapters, catalog and panel | Design only. Starter DB assumes an existing host; Vaultix compose is manual; `service_instances` is rrflow-specific. | Configurable engine/version/image, placement, storage, compute, network, credentials, backup/restore, upgrades and health. Redis compatibility must be an explicit engine choice, not a false interchangeable promise. Test restore and tenant isolation. |
| Cloudflare Tunnel, Tailscale and custom connectivity | `internal/networking`, `shippin_network_*` tables, infrastructure UI | Configured integrations store secret ciphertext separately, expose CRUD and refresh bounded inventory. Provider fetch is observational inventory, not tunnel/device lifecycle. | Add/test/rotate/revoke and only provider mutations supported by least-privilege scopes. Attach resources to stable systems; label observation separately from managed state. Custom integration stays schema-bounded. |
| Platform service registry and health | Existing platform-service API/probe and overview UI owners | Probe definitions currently come from configuration; cards can show probes but there is no complete add/edit persistent registry for Nomen, Vaultix, Edge, Database, Operator and user services. | Persist validated probe/service definitions; configure addresses, auth refs, type, timeout/interval and alias; report fresh latency/resource observations and distinct unknown/stale/down states. Never display hardcoded endpoints or fake zero metrics. |
| Resource dashboards and events | Infrastructure, managed-service and platform health owners | Hypervisor node/guest data and operation history exist in separate flows. No unified actual deployment/resource view. | Join only by stable IDs; preserve observation source/time; expose per-owner health, capacity, deployment events and safe detail links. Do not mix desired state with observed state. |
| Backups, DNS, firewall, ACME, logs, packages and disk controls | Existing provider/networking/managed-service owners as appropriate | Warden matrix identifies remaining author/review items; some Proxmox reads/snapshots exist, but estate backup executors and broad host controls are not complete. | Re-evaluate each matrix row against current platform routes and tests. Author only bounded supported capabilities; reject host shell/process controls and unsafe site-specific behavior. |
| Persistent schema and secret handling | `internal/db`, owning bounded context and configured Vaultix/sops integration | New network tables are prefixed. Historical catalog/server/service tables are unprefixed. Source metadata holds credential references only. | All new tables use `shippin_`; do not rename old tables without additive migration, compatibility, backfill, rollback and restore evidence. Never store or return secret bytes in application records. |
| Developer build and rollout | `cmd/console`, `cmd/shippin-hostd`, `web/`, install/compose | Current CI passes install/compose and builds/tests Linux Go and React. Hostd still requires manual deployment in documented flow. | Verify standalone artifacts, configuration-only install, host enrollment, migration, upgrade/backup/restore and actual provider actions from clean main. Update roadmap with proven and unproven operations. |

## Mesh and Warden source feature families

The source paths below are feature evidence from `shippin-mesh` commit
`26cce35b`; they are not accepted imports. The current platform owner column
names the destination contract, not proof every behavior is present.

| Source family | Useful behavior to compare | Platform destination and present gap |
|---|---|---|
| `warden/proxmox.go`, `warden/internal/host/kinds/proxmox/`, `docs/specs/02-proxmox-connector.md`, `agent/clyffy-dean/` | Node/guest inventory, actions, task tracking, snapshots, storage and API privilege boundaries | `internal/hv/proxmox`, `internal/api/infra*`, Shippin Prox UI. Compare each operation and unsupported guest type. Source doc is single-node, does not poll UPIDs and says TLS verification defaults off; none of these shortcuts carry forward. |
| `warden/catalog.go`, `warden/deployments*.go`, `templates/`, `docs/specs/07-dev-portal.md` | Template catalog, declared inputs, placement, deployment stages, observed status, events and lifecycle | `internal/catalog`, new deployment bounded context and existing panel. Current catalog saves metadata only; there is no generic executor/reconciler. File templates and arbitrary shell/Compose must pass provenance, license, dependency, isolation and privilege review. |
| `warden/backups*.go`, `warden/estate.go`, backup templates | S3/SMB/object-store destination config, coverage, schedule, events, dump/restore workflows | `internal/backupcov`, hypervisor snapshot adapters and future managed-service backup owner. Current code does not provide estate copy/restore executors. Secret-file storage and site paths are rejected; backup restore proof is required. |
| `warden/dns.go`, `warden/domain_registry.go`, `warden/certs.go`, `warden/endpoints.go` | Domain ownership, records, TLS/ACME state, route and endpoint observations | Existing workspace/domain/API and networking owners. DNS and ACME create/update operations need explicit provider capability, least-privilege credentials, domain ownership validation and read-back; do not add host-file editing. |
| `warden/network.go`, `warden/firewall.go`, `registry/`, `docs/specs/04-edge-and-routing.md` | Provider network inventory, route, firewall and service-level reachability | `internal/networking`, `internal/hv/{incus,proxmox}` and health registry. Existing network integrations observe Cloudflare/Tailscale/custom resources; provider mutation, firewall policy and service attachment are not implied. |
| `warden/disks.go`, `warden/docker.go`, `warden/sysactions.go`, `warden/packages.go` | Storage inventory and bounded host service/package operations | Hypervisor storage capability and managed-service/provider adapters. Root-shell, arbitrary Docker, process-kill, local package mutation and destructive disk controls are not imported; only typed least-privilege operations with provider checks can be authored. |
| `warden/ci_runners*.go`, `warden/ci_stats*.go`, `warden/runners_page.go` | Runner registration, status, capacity and CI metrics | Existing agent/job and project owners. No CI runner fleet API is proven in Shippin. Add only after a tenant/security boundary and credential owner are defined. |
| `warden/customers*.go`, `warden/managed_workspaces*.go` | Customer ownership, assigned resources and service view | Existing tenant, org, project and workspace contexts. Do not create a second customer or service registry; map customer access to stable Shippin tenant/system IDs. |
| `warden/platform_systems*.go`, `warden/health_widgets.go`, `warden/sysinfo.go` | Platform process/host probes and operator health cards | Existing overview probe plus planned dynamic service registry. Current probes are configured/static and do not provide add/edit, fresh resource telemetry or managed service lifecycle. |
| `warden/logs.go`, `warden/filemanager.go`, `warden/terminal*.go`, `warden/sshkeys.go`, `warden/users.go` | Host diagnostics, file access and remote operation surfaces | Existing audit, workspace/terminal, SSH-key and identity owners. Host-root file manager, shell, local accounts and unsanitized logs are rejected; any diagnostic must be scoped, read-only where possible, redacted and audited. |
| `warden/internal/client/`, client YAML fixtures, `registry/context-mesh.yaml`, specialist/agent modules | Tenant client policies, context/MCP registries and AI tool composition | AgentHub owns human review; Shippin owns system inventory and provider access. Do not import a second policy/registry or site-defined tenant IDs. Only a documented authenticated API adapter is in scope. |
| `warden/oidc*.go`, `authentik/`, `templates/authentik*`, `agent/` identity sidecars | Historical IdP and provisioning templates | `internal/auth` and configured Nomen. No PAM/local Warden user store or parallel customer panel. Templates need upstream license, current image pin and isolation review before any adoption. |
| `surreal/`, `supabase/`, `intelligence/`, Qdrant/Surreal templates and Clyffy specialists | AI memory/graph/search experiments and private runtime integrations | Outside the requested control-plane/service deployment absorption unless the product ledger assigns a concrete Shippin capability. Preserve as separate source context; do not pull their databases, IDs, prompts or credentials into platform truth. |

This family map is a review index, not a blanket disposition. Each family stays
open until its source behavior, current platform implementation, user-facing
workflow, tests, security/rights and failure path have been compared and recorded.

## Warden file-matrix reconciliation

`docs/design/warden-capture-matrix.md` has 216 imported-source rows for the 188
tracked files plus 28 absent screenshots, counted as 6 ported, 13 to-author, 176
intentionally replaced and 21 rejected. Five separately listed mesh-source rows
remain `needs-review`; they are not included in those 216 rows or status counts.
Correct these counts/statuses only when rechecking the rows, not by treating
screenshot omissions as source code. The five mesh documents listed at the end are readable in local `shippin-mesh` commit
`26cce35b`; the old GitHub-404 description is obsolete. The matrix's Gold-era
approval wording also conflicts with current main and must be replaced with the
AgentHub ownership boundary.

Do not mark the imported tree archive-ready until each remaining row has current
platform route/API/schema/test evidence or a documented rejection. Keep the source
tree and external worktrees until the user reviews the completed archive checklist.

## Execution sequence

The full target architecture, seven implementation phases and exit gates are in
[the platform absorption plan](platform-absorption-plan.md). The ignored,
schema-validated session plan `shippin-configurable-platform-2026-11` tracks the
current execution step, source hashes and verification evidence. Work proceeds
in phase order through direct commits to `main`; a passing build does not make
unrelated ledger rows done.
