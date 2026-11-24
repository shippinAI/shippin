# Code map

How the code is organized, how far it is from that today, and the order it gets
rewritten in. [What Shippin is](objective.md) defines the product; this page says
where each part of it lives. The measurements below are historical, from 2026-11-10.
The [foundation delivery plan](foundation-delivery.md) grounds the next packets in
the 2026-11-20 source and connects these contexts to navigation and behavior checks.

## Contexts

Each part of the product is one context, and one owner in the code.

| Context | Owns | Go packages, all under `internal/` | Panel feature |
| --- | --- | --- | --- |
| Access | Who is calling, sessions, permissions, the audit log | `access/authz`; sign-in and sessions are `auth` | shared |
| Identity | Accounts, passkeys, sign-in providers, staff roles, teams | `identity`, with `agents`, `devaccounts`, `access_key`, `ssh_key` | `identity`, `team` |
| Organizations | Organizations, team invitations, workspaces, projects | `tenancy`, with `orgs`, `projects`, and the enterprise entities until they leave | `organizations`, `workspaces`, `projects` |
| Infrastructure | Providers (Incus, Proxmox, public clouds, customer-brought servers), regions, hosts, guests, customer machines, networks, fleets, capacity, metrics | `infrastructure`, with `servers`, `enroll`, `healthprobe`, `networking`, `hostd`, `hostclient`, `metric`, `host_detection`; the drivers are `hv` | `infrastructure` |
| Services | Catalog, deployments, managed services, jobs | `services`, with `catalog`, `deployments`, `managed`, `jobs`, `tenantdb`, `catalog_service`, `runner_size` | `catalog`, `deployments` |
| Runtime | Development workspaces on the owner's hosts, terminals and their plain-language layer, desktops, bridges, devices and project sync, AI subscriptions, chat | `runtime`, with `ailaunch`, `apps`, `bridge`, `chat`, `credwriter`, `hub`, `approval`, `agentruns`, `sshca`, `subs`, `runner`, `runner_job` | `devspaces`, `terminal`, `chat`, `connections` |
| Platform | Settings, interface, health, updates, setup | `platform`, with `config`, `version`, `spa`, `tlsfile`, `mail`, `backupcov`, `setting`, `secret_store`, `job_queue` | `settings` |
| Integrations | GitHub, Vaultix, rrflow, AgentHub, each optional | `integrations`, with `gh`, `vaultix`, `vaultixhost`, `rrflow`, `agenthub` and the GitHub entities | under the context that uses it |

The policies a module can replace sit with the seam: `ext/limits` and `ext/placement`.

Every package was moved into its context on 2026-11-24, unchanged, so the folders match this table. Two shared layers remain and are the rewrite below: `internal/api`, the one HTTP package every legacy handler still hangs off, and `internal/db`, the storage they share. A handler leaves `internal/api` by becoming an action file in its entity's folder.

These are plumbing, shared by every context and owned by none: `internal/app` (start-up), `internal/mcpl` and `internal/actions` (the action runner and its generated registry), `internal/ext` (the extension seam), `internal/ports` (host ports), `internal/settings` (the settings sections each package declares; the Platform context's `setting` entity edits them), `internal/clickhouse` and the database connection.

Nothing else is added beside them. New code is an entity folder inside its context, and a child of an existing entity is a folder inside that entity. `TestInternalPackagesAreContextsOrListed` in `internal/mcpl` fails on a new top-level package; `internal/mcpl/testdata/unplaced-packages.txt` names what is still outside a context, which is `internal/api` alone, and only shrinks. The panel has the same shape: `web/src/features/` holds one folder per context, a feature is a folder inside one, and its boundary check refuses anything else ([panel framework](web-framework.md)).

## Editions

The contexts above are the panel. The enterprise module ([Editions and licensing](editions.md)) is a second tree with the same shape, in the private repository `shippinAI/shippin-enterprise` under the Go module `shippin.cloud/console/enterprise`. It holds:
- **Customers:** customer organizations, plans, coverage tiers, quotas, usage and billing, sign-up, the waitlist and acquisition.
- **Customer access:** access grants, support access, break-glass and staff mode.
- **Isolation:** `hostd`, the per-customer host agent (encrypted storage, private networks, VM boundaries), tenant databases and hosted Vaultix.

The module plugs into the panel through `internal/ext`:
- start-up through `app.Main` in `internal/app`;
- generated action registries, merged into the panel's;
- settings sections, permissions and job kinds;
- its own migrations, run after the panel's;
- the caller, organization lifecycle, limits and placement policies;
- panel pages under `web/src/modules/`.

Code in this repository never imports the module. A context that needs a business rule asks a policy, and the panel's default answers it for a self-hosted installation.

Today the enterprise code still sits in this repository, mixed into these contexts: `internal/tenancy/{plan,coverage,coverage_tier,customer,acquisition_source}`, `internal/tenancy/acquisition`, `internal/services/tenantdb`, `internal/integrations/vaultixhost`, `internal/infrastructure/hostd`, and the tenant, access, sign-up and onboarding handlers in `internal/api`. The [roadmap](roadmap.md#separation-first) moves it out before broad context rewrites; P1/P2 complete the immediate deploy-and-claim foundation in existing owners.

## The shape of a context

This is the target pattern. Existing MCPL entities such as
`internal/platform/setting` demonstrate it; legacy context code still needs migration:

- A context owns its types, rules, storage and transport through cohesive entity packages.
- Each entity is a folder, `internal/<domain>/<entity>/`, holding its types and storage (`<entity>.go`), one file per action (`<verb>.go`) and its tests beside them. The folder and file give the action its [MCPL](mcpl.md) ID and route; `go generate ./internal/actions` writes the registry, the panel's typed action map and the OpenAPI document, and `cmd/console` mounts them; `go run ./cmd/mcplgen new` starts a new one. Nothing lists routes by hand.
- `internal/tenancy/coverage_tier` is the first entity in this shape; the rest of `internal/services/catalog` follows it.
- It takes what it needs through a small `Deps` struct, and never reaches into another context's tables.
- It calls another context only through that context's exported functions.
- Its panel code is its context's folder in `web/src/features/`, one folder per feature inside it, and its API is its own file under `api/openapi/`.

## Where it stands

`internal/api` is one package of 186 files and 22,382 lines. Every handler hangs off
one `API` struct with 46 fields, so any handler can reach anything. Measured by
file name, which is a first pass:

| Context | Files | Lines | Routes | Calls into others | Called by others |
| --- | --- | --- | --- | --- | --- |
| Identity and access | 45 | 4,590 | 47 | 20 | 94 |
| Services | 28 | 4,382 | 19 | 69 | 26 |
| Infrastructure | 12 | 3,798 | 70 | 23 | 32 |
| Runtime | 37 | 3,630 | 35 | 48 | 49 |
| Platform | 24 | 2,430 | 45 | 36 | 42 |
| Tenancy | 21 | 2,330 | 26 | 45 | 15 |
| Integrations | 9 | 678 | 6 | 11 | 9 |
| HTTP shell | 9 | 662 | 40 | 26 | 17 |

`internal/db` is a second shared layer: 33 files of storage for every context.
`web/src/features/platform/admin` holds 74 files and 4,068 lines that belong to other
contexts. `deploy/` still holds scripts from when one site was run by hand.

`internal/platform` is the first package written to the target shape: built-in services, setup steps and disaster recovery.

## Order of the rewrite

After P1/P2, the [separation](roadmap.md#separation-first) precedes broad context rewrites, so no context is rewritten twice. Then one context at a time. Each step moves the handlers and storage into the context
and rewrites what is wrong rather than carrying it over. The routes stay the same
unless the step says otherwise, and each step ends by overwriting the review deploy.

1. **Access.** What 94 calls depend on: caller, permissions and audit. Everything else builds on it.
2. **Infrastructure,** with the dashboards, which are the next feature.
3. **Platform,** with every setting moving into the panel.
4. **Identity.**
5. **Organizations,** once plans, coverage and subscriptions have moved to the enterprise module.
6. **Services,** with PostgreSQL and ClickHouse as the first catalog entries.
7. **Runtime.**
8. **Integrations,** then the empty `internal/api` shell and `internal/db` are removed, and `deploy/` is cleared of what the panel now does.
