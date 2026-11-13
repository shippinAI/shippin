# MCPL in Shippin

MCPL is the owner's framework for letting an AI operate an app as the person it works for. The person
explains what they want in a natural chat. The AI then acts through the same actions the person would
use, with the same permissions and audit. Plain code does the mechanical work so that the model's
tokens go to choices. Scripted events tell the AI when to step in, and on screen the panel moves the
way a person would. Each of the owner's apps builds its own variant so the standard can be drawn from
real ones. This is Shippin's. It is contract rule MCPL-01 in [What Shippin is](objective.md).

## Where an action lives decides what it is

An action is one file. Its folder and file name are its identity, and everything else is derived from
them:

```text
internal/<domain>/<entity>/<verb>.go          one action
internal/<domain>/<entity>/<entity>.go        the entity's types, storage, rules, shared lookups
internal/<domain>/<entity>/*_test.go          its tests, beside it
internal/<domain>/<entity>/<child>/<verb>.go  a nested entity: /<entities>/{<entity>Id}/<children>
```

`internal/tenancy/coverage_tier/update.go` declares

```go
var Update = mcpl.Def[UpdateInput, Tier]{
	Spec: mcpl.Spec{Summary: "Change a coverage tier's support, services, sign-in rule or price.", Access: mcpl.Staff, Perm: "catalog.plans.write"},
	Run:  func(c *mcpl.Call, in UpdateInput) (Tier, error) { ... },
}
```

and from that location alone it is:

| | Derived |
| --- | --- |
| ID | `shippin.tenancy.coverage_tier.update` |
| Route | `PUT /v1/operator/tenancy/coverage-tiers/{coverageTierId}` |
| Risk | write: it changes something, so it needs a reason |
| Registry entry | `internal/actions/registry_gen.go` |
| Panel entry | `web/src/lib/api/actions.gen.ts`, called as `callAction("shippin.tenancy.coverage_tier.update", { coverageTierId }, input)` |
| Manifest | `GET /v1/operator/mcpl` |

`go generate ./internal/actions` (`cmd/mcplgen`) finds every action file and writes the registry and
the panel map. No route, ID or path is written by hand anywhere. `internal/actions` fails its tests
when the generated files are stale, so a new or moved action cannot ship without them.

### Route rules

- **Audience** gives the prefix:
  - staff: `/v1/operator`;
  - an organization's members: `/v1/tenants/{tenantId}`;
  - one's own account: `/v1/me`;
  - public: `/v1`.

  The domain follows the prefix.
- **Entities** are plural and kebab-case. An outer entity is followed by its own `{<entity>Id}`.
- **Verbs** map to methods:
  - `list` GETs the collection and `create` POSTs to it.
  - `get` GETs one item, `update` PUTs it, and `delete`, `remove` and `revoke` DELETE it.
  - Any other verb POSTs to the item, or to the collection when the spec says `Collection`.
- **Risk** follows the method: a GET reads, a DELETE destroys, everything else writes.

### What every action gets without writing it

`mcpl.Def` runs every action the same way.

#### Input

- The body is a JSON object decoded strictly: an unknown field is refused, and so is an `id` in the
  body of an update, because the path carries it.
- Path and query values fill the fields tagged `path:"..."` and `query:"..."`.

#### Reason

- Every staff change needs `reason` in the body, for DELETE too.

#### Transaction and audit

- A change runs in one database transaction, as `c.DB`.
- The audit entry (action ID, item and reason) is written after the action succeeds. The transaction
  commits only if that entry is saved, so a refused attempt is never logged as a change, and a change
  is never made without its entry.

#### Output

- A list is `mcpl.Items` (`{"items": [...]}`), `create` answers 201, and `mcpl.None` answers 204.

#### Errors

- Errors are `application/problem+json` with a plain `message`:
  - `mcpl.Invalid` 400, `Forbidden` 403, `NotFound` 404 and `Conflict` 409;
  - a missing row 404, a duplicate 409;
  - anything unexpected 503, logged.

#### Permissions

- The route is gated by `Perm` like every operator route.
- `c.Can(perm)` answers field-level questions, such as hiding prices from a caller who may not see
  them.

The spec's values are literals, so the generator reads them without running code.

## Domains

`public`, `auth`, `access`, `identity`, `tenancy`, `infrastructure`, `services`, `runtime`,
`platform`, `integrations`: the contexts of the [code map](architecture.md), plus `public` and `auth`.
The same list names action IDs, events, UI actions and, later, the per-domain MCPs.

## Where it stands, 2026-11-13

| Piece | State |
| --- | --- |
| Generated actions | `coverage_tier` (list, get, create, update, delete), proved with a database test and in a browser. |
| Legacy routes | 343 served `/v1` routes are not yet action files. `internal/api/testdata/mcpl-undescribed.txt` lists them; `TestMCPLConformance` fails on a new one and on a stale entry, so the list only shrinks. |
| Mounting | Staff actions are mounted. Public, signed-in and member actions are refused at startup until their mounting is written. |
| Manifests | `GET /v1/mcpl` (public) and `GET /v1/operator/mcpl` (staff, with what the caller may run). |
| MCP | Planned: generated from the registry, one MCP per domain, running the same `Def`. Input schemas come from the `In` types. |
| Events | Planned: `shippin.<domain>.<entity>.<past verb>`, emitted where the audit entry is written today. |
| UI actions, components, lessons | Planned: `shippin.ui.<domain>.<workflow>.<action>` with readiness and receipts. The panel's controls have roles and labels, which browser tests already drive. |

Measured on 2026-11-12, before any entity moved:
- six staff prefixes;
- `{id}` and one-letter path parameters;
- five routes hiding several actions behind `{action}`;
- 34 handlers ignoring unknown fields;
- the reason sent both in the body and in an `X-Change-Note` header;
- three permission models;
- audit names in mixed tenses.

Each disappears as its routes become action files.

## Order of work

1. **Done.** The action runner, the generator, the registry and the panel map, the conformance
   ratchet, and the coverage tier as the first entity, end to end.
2. **Entity by entity, in the [code map](architecture.md)'s order.** Move each into
   `internal/<domain>/<entity>/`, switch its panel calls to `callAction`, and shrink the list in the
   same commit. Plans, catalog services and customers come next, then Access.
3. **MCP from the registry,** with a person's delegated session as how an AI signs in.
4. **Events,** then **UI actions** with receipts, proved in a real browser.

## Adding an action

1. Pick the domain and the entity folder. If none fits, it is probably two actions.
2. Write `<verb>.go` with `var <Verb> = mcpl.Def[In, Out]{Spec: ..., Run: ...}`, its tests beside it.
3. Run `go generate ./internal/actions`.
4. In the panel, call it with `callAction("<id>", params, input)`.
