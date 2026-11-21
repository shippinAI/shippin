# Code map

How the code is organized, how far it is from that today, and the order it gets
rewritten in. [What Shippin is](objective.md) defines the product; this page says
where each part of it lives. The measurements below are historical, from 2026-11-10.
The [foundation delivery plan](foundation-delivery.md) grounds the next packets in
the 2026-11-20 source and connects these contexts to navigation and behavior checks.

## Contexts

Each part of the product is one context, and one owner in the code.

| Context | Owns | Go package | Panel feature |
| --- | --- | --- | --- |
| Access | Who is calling, sessions, permissions, the audit log | `internal/access` | shared |
| Identity | Accounts, passkeys, sign-in providers, staff roles, teams | `internal/identity` | `identity`, `team` |
| Organizations | Organizations, team invitations, workspaces, projects | `internal/tenancy` | `organizations`, `workspaces`, `projects` |
| Infrastructure | Providers (Incus, Proxmox, public clouds, customer-brought servers), regions, hosts, guests, customer machines, networks, fleets, capacity, metrics | `internal/infrastructure`, `internal/hv` | `infrastructure` |
| Services | Catalog, deployments, managed services, jobs | `internal/services` | `catalog`, `deployments` |
| Runtime | Development workspaces on the owner's hosts, terminals and their plain-language layer, desktops, bridges, devices and project sync, AI subscriptions, chat | `internal/runtime` | `devspaces`, `terminal`, `chat`, `connections` |
| Platform | Settings, interface, health, updates, setup | `internal/platform` | `settings` |
| Integrations | GitHub, Vaultix, rrflow, AgentHub, each optional | one package each | under the context that uses it |

`internal/clickhouse` and the database connection are plumbing, shared by contexts.

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

Today the enterprise code still sits in this repository, mixed into these contexts: `internal/tenancy/{plan,coverage,coverage_tier,customer,acquisition_source}`, `internal/acquisition`, `internal/tenantdb`, `internal/vaultixhost`, `internal/hostd`, and the tenant, access, sign-up and onboarding handlers in `internal/api`. The [roadmap](roadmap.md#separation-first) moves it out before broad context rewrites; P1/P2 complete the immediate deploy-and-claim foundation in existing owners.

## The shape of a context

This is the target pattern. Existing MCPL entities such as
`internal/platform/setting` demonstrate it; legacy context code still needs migration:

- A context owns its types, rules, storage and transport through cohesive entity packages.
- Each entity is a folder, `internal/<domain>/<entity>/`, holding its types and storage (`<entity>.go`), one file per action (`<verb>.go`) and its tests beside them. The folder and file give the action its [MCPL](mcpl.md) ID and route; `go generate ./internal/actions` writes the registry, the panel's typed action map and the OpenAPI document, and `cmd/console` mounts them; `go run ./cmd/mcplgen new` starts a new one. Nothing lists routes by hand.
- `internal/tenancy/coverage_tier` is the first entity in this shape; the rest of `internal/catalog` follows it.
- It takes what it needs through a small `Deps` struct, and never reaches into another context's tables.
- It calls another context only through that context's exported functions.
- Its panel code is one folder in `web/src/features/`, and its API is its own file under `api/openapi/`.

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
`web/src/features/admin` holds 74 files and 4,068 lines that belong to other
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
