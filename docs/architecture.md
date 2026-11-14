# Code map

How the code is organized, how far it is from that today, and the order it gets
rewritten in. [What Shippin is](objective.md) defines the product; this page says
where each part of it lives. Measured 2026-11-10.

## Contexts

Each part of the product is one context, and one owner in the code.

| Context | Owns | Go package | Panel feature |
| --- | --- | --- | --- |
| Access | Who is calling, sessions, permissions, the audit log | `internal/access` | shared |
| Identity | Accounts, passkeys, sign-in providers, staff roles, teams | `internal/identity` | `identity`, `team` |
| Tenancy | Organizations, invitations, workspaces, projects, plans, coverage, subscriptions, usage | `internal/tenancy` | `organizations`, `workspaces`, `projects`, `plans` |
| Infrastructure | Providers (Incus, Proxmox, public clouds, customer-brought servers), regions, hosts, guests, customer machines, networks, fleets, capacity, metrics | `internal/infrastructure`, `internal/hv` | `infrastructure` |
| Services | Catalog, deployments, managed services, jobs | `internal/services` | `catalog`, `deployments` |
| Runtime | Devspaces, terminals and their plain-language layer, desktops, bridges, devices and project sync, AI subscriptions, chat | `internal/runtime` | `devspaces`, `terminal`, `chat`, `connections` |
| Platform | Settings, interface, health, updates, setup | `internal/platform` | `settings` |
| Integrations | GitHub, Vaultix, rrflow, AgentHub, each optional | one package each | under the context that uses it |

`internal/hostd` is the host agent and stays its own program. `internal/clickhouse`
and the database connection are plumbing, shared by contexts.

## The shape of a context

`internal/catalog` already works this way, and it is the pattern to follow:

- One package holds a context's types, rules, storage and HTTP handlers.
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

One context at a time. Each step moves the handlers and storage into the context
and rewrites what is wrong rather than carrying it over. The routes stay the same
unless the step says otherwise, and each step ends by overwriting the review deploy.

1. **Access.** What 94 calls depend on: caller, permissions and audit. Everything else builds on it.
2. **Infrastructure,** with the dashboards, which are the next feature.
3. **Platform,** with every setting moving into the panel.
4. **Identity.**
5. **Tenancy,** with plans and subscriptions.
6. **Services,** with PostgreSQL and ClickHouse as the first catalog entries.
7. **Runtime.**
8. **Integrations,** then the empty `internal/api` shell and `internal/db` are removed, and `deploy/` is cleared of what the panel now does.
