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
- **Singletons.** An entity there is one of in its parent (an organization's coverage, a workspace's
  firewall) says `Singleton`. Its path is singular with no item ID:
  `internal/tenancy/coverage/get.go` is `GET /v1/tenants/{tenantId}/tenancy/coverage`.
- **Risk** follows the method: a GET reads, a DELETE destroys, everything else writes.

### Who may call it

`Access` and `Perm` decide, through the same gate the routes before actions used:

| Access | Gate | `Perm` |
| --- | --- | --- |
| `Staff` | the operator gate: origin, sign-in, staff session rules | the catalog permission |
| `Member` | the organization in `{tenantId}`, which the caller belongs to or holds an access grant for | `""` anyone who sees it, `"manage"` a manager, `"decide"` an owner or admin |
| `SignedIn` | any session; the action runs as `c.Actor` | none |
| `Public` | none; only `public` and `auth` actions may change anything | none |

A member action runs with `c.Tenant` set. A change made through a staff access grant is marked as such
in the organization's audit log.

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
- The audit entry (action ID, what it acted on, and the reason) is written on that same transaction
  after the action succeeds. The entry and the change commit together or not at all, so a refused
  attempt is never logged as a change, and a change is never made without its entry.

#### Events

- Every change emits an event once it is committed: the action's ID with the verb in the past,
  `shippin.identity.ssh_key.create` emits `shippin.identity.ssh_key.created`.
- An event says who, in which organization, and what it acted on (`subject`: the route's IDs and the
  created item's `id`). It never carries the action's output; a follower reads the item through a get
  action, under its own permissions.
- Inside the panel's server, `a.events.On(id, fn)` reacts to one. Pushing a person's SSH keys to their
  running workspaces is such a reaction, not part of the action.
- `GET /v1/me/mcpl/events` streams them (server-sent events) to the people who could have run the
  action: their own, their organizations' at their role, and staff events their permissions cover.

#### Services

- An action uses nothing but `c.DB` directly. Anything else (mail, the identity provider, a
  hypervisor) is an interface its own package declares, fetched with `mcpl.Need[T](c)`.
- `internal/api/mcpl_services.go` provides the implementations. Tests hand in fakes through
  `mcpltest.As{Services: ...}`. An install without the service answers a plain 503.

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

### Routes that are not actions

A stream, a WebSocket, a browser redirect in a sign-in flow, a webhook another system sends, the
edge's forward-auth check, a file download and another protocol's endpoint (MCP) are not actions.
They are mounted with `a.transport(mux, pattern, Carries, handler)`, which records what they carry.
The conformance test accepts them as such and nothing else.

### Testing an entity

`internal/mcpl/mcpltest` serves an entity's generated actions on their routes against a fresh test
database, as a caller you set, and collects the audit entries and events:

```go
h := mcpltest.Serve(t, actions.All, "shippin.identity.ssh_key.", mcpltest.As{Actor: "ada"})
k := h.Call("POST", "/v1/me/identity/ssh-keys", body, 201)
```

## MCP

`POST /v1/mcp` is an MCP server (streamable HTTP, JSON responses) whose tools are the generated actions
the caller may run, and nothing else. `POST /v1/mcp/<domain>` serves one domain's tools.

- **Names.** A tool is the action ID without `shippin.` and with dashes: `identity-ssh_key-create`.
- **Input.** Its input schema is derived from the action's `In` type, plus its route's parameters and,
  for a staff change, the required `reason`. A member tool lists the caller's own organizations as the
  choices for `tenantId`.
- **Hints.** Reads are marked read-only, deletes destructive.
- **Dispatch.** A call is dispatched as an ordinary request to the action's own route, with the caller's
  own credentials. It passes the same gate, takes the same reason, commits with the same audit entry
  and emits the same event as the panel's call. Nothing is reached around the route.

### Access keys

An AI client signs in with an access key: **Account > AI access** in the panel, or the
`identity.access_key` actions.
- A key acts as the person who made it, through their own permissions.
- Only its hash is stored, and it is shown once.
- It expires within 90 days, shows when it was last used, and is revoked at once.
- It carries the sign-in method of the session that made it, so an organization's sign-in rule still
  applies.
- A key cannot make another key.
- Staff routes that need a recent sign-in refuse it, so those stay a person's own session.
- Taking over the setup account revokes the keys made with it.

```sh
claude mcp add --transport http shippin https://<panel>/v1/mcp --header "Authorization: Bearer shp_..."
```

## Domains

`public`, `auth`, `access`, `identity`, `tenancy`, `infrastructure`, `services`, `runtime`,
`platform`, `integrations`: the contexts of the [code map](architecture.md), plus `public` and `auth`.
The same list names action IDs, events, UI actions and, later, the per-domain MCPs.

## Where it stands, 2026-11-13

| Piece | State |
| --- | --- |
| Generated actions | Staff: `tenancy.coverage_tier` (list, get, create, update, delete). Member: `tenancy.coverage` (get). Signed-in: `identity.ssh_key` (list, create, delete). Each has a database test. |
| Legacy routes | 340 served `/v1` routes are not yet action files or declared transports. `internal/api/testdata/mcpl-undescribed.txt` lists them; `TestMCPLConformance` fails on a new one and on a stale entry, so the list only shrinks. |
| Mounting | Every audience: staff, member, signed-in and public. |
| Events | Emitted by every change after commit, with in-process reactions and a stream at `GET /v1/me/mcpl/events`. |
| Manifests | `GET /v1/mcpl` (public) and `GET /v1/operator/mcpl` (staff, with what the caller may run). |
| MCP | `POST /v1/mcp` and `POST /v1/mcp/<domain>`: tools generated from the registry, dispatched through each action's own route. Access keys for AI clients, in the panel under **Account > AI access**. |
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
   ratchet, and the coverage tier as the first entity, end to end. Then every audience, events,
   services and transports, with SSH keys and an organization's coverage moved.
2. **Entity by entity, in the [code map](architecture.md)'s order.** Move each into
   `internal/<domain>/<entity>/`, switch its panel calls to `callAction`, and shrink the list in the
   same commit. Plans, catalog services and customers come next, then Access.
3. **Done.** MCP from the registry, with access keys as how an AI client signs in as a person.
4. **UI actions** with receipts, proved in a real browser.

## Adding an action

1. Pick the domain and the entity folder. If none fits, it is probably two actions.
2. Write `<verb>.go` with `var <Verb> = mcpl.Def[In, Out]{Spec: ..., Run: ...}`, its tests beside it.
3. Run `go generate ./internal/actions`.
4. In the panel, call it with `callAction("<id>", params, input)`.
