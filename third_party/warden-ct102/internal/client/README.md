# warden/internal/client/

Multi-tenant (client) abstraction. See
[`docs/decisions/0008-repo-foundation.md`](../../../docs/decisions/0008-repo-foundation.md)
§4.

A `Client` is a tenant: it owns one or more Hosts, has a tier, and a
policy that gates what it can do. The system itself is the implicit
"system" tenant — its hosts bypass per-client policy.

## Files

- [`client.go`](./client.go) — `Client`, `Tier`, system-tenant rules
- [`registry.go`](./registry.go) — load + validate `clients.yaml` and per-client detail
- [`policy.go`](./policy.go) — `Capability`, `Policy`, system policy default

## On-disk layout per client

```
/etc/warden/clients/<slug>/
├── client.yaml         # name, owner_email, tier, hosts_owned, policy_ref
├── policy.yaml         # explicit capability grants + host scoping
└── reports/            # generated billing / usage / audit reports
```

## Tier model

| Tier | Council size | Memory routing (ADR 0009) | Manual gate strictness |
|---|---|---|---|
| `default` | 1 (single Clyffy) | always-Postgres for all kinds | thresholds at defaults |
| `enhanced` | 2-3 specialists | always-Postgres + live SurrealDB views | defaults |
| `premium` | full council | mode III for `hypothesis` / `deliberation` / `trace` / `computed_edge` | per-tenant override allowed |

## Phase status

Phase 1 (this commit) ships the types + registry + permissive system
policy. Phase 3 wires real capability checks at every mutation boundary
(deploy, host write, memory.write, etc.).
