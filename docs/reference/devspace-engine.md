# The development space engine

A plan, not built. It re-authors how a development space is made, opened and removed so that there
is one engine for guests on a host, behind the hypervisor drivers, and the panel only shows what
that engine says. It serves WS-01, TERM-01, IDE-01, INF-03, MCPL-01 and ENG-01 in
[What Shippin is](../objective.md), and steps 1 and 2 of the current target in
[the hosting coverage plan](../hosting-coverage.md).

Read in the code on 2026-11-24. Two things below are assumed and must be checked on the rigs
before the packet that needs them: that Incus's instance exec endpoint gives an interactive
terminal over a WebSocket with resize, and what Proxmox offers for the same on a container.

## What is wrong today

Four separate paths make a guest on a host, and none shares code with another:

| Path | Used for | Goes through |
| --- | --- | --- |
| Admin guests | Staff creating and powering machines | `internal/api/infra*.go` to `internal/hv` |
| Runner machines | One VM per build job | `ports.Machines`, provided by `internal/api/runners.go`, to `internal/hv` |
| Catalog deployments | An application container | `internal/services/deployments` to Incus-only calls in `internal/hv/incus/oci_application*.go` |
| Development spaces | Everything a person builds in | `internal/infrastructure/hostclient` to one `hostd` address, which runs the `incus` command line |

The fourth is the one a customer lives in, and it is the one that skips the drivers. It knows one
host, only Incus, takes its sizes from a file of its own, keeps provisioning progress in memory,
and will not open a terminal unless two unrelated sections (AI and Connect) are configured, because
the user it runs as is stored in the AI section.

## The decision

**One guest engine.** Every guest on every host is made, observed, entered and removed through one
port, `ports.Guests`, backed by `internal/hv`. Runner machines move onto it first because they
already use the drivers; development spaces are written on it; admin guests and deployments follow.
`hostd` is no longer in the development space's path. What it does for per-customer isolation is
enterprise code and leaves with it.

**The name is devspace.** The contract says "development workspace", and the panel already has
"workspaces" meaning a group of projects. Two things called workspace with two kinds of ID is the
confusion in today's routes. The runtime entity is `devspace` everywhere: folder, action IDs,
routes, tables, panel feature.

**The panel decides nothing.** Every read returns the state, the steps so far and the actions that
are possible now, each with a reason when it is not. The panel draws that and calls the generated
action. No page holds a rule about what is allowed.

## Where the code lives

Each folder is one entity. One file is one action, with its test beside it. The panel folder has
the same name as the entity it shows.

```text
internal/hv/
  driver.go                     adds the optional contracts below
  incus/exec.go                 Executor for Incus
  incus/terminal.go             Terminal for Incus
  proxmox/exec.go, terminal.go  the same contracts for Proxmox containers

internal/ports/guests.go        the one port every guest goes through

internal/runtime/
  devspace/
    devspace.go                 the type, its states, storage, lookups, what is allowed now
    create.go  get.go  list.go  start.go  stop.go  delete.go
    provision.go                the job: its steps, each safe to run again
    settings.go                 base domain, idle stop, the default template
    services.go                 what the entity needs: ports.Guests, the limits policy
    give_keys.go                reaction: a person's SSH keys reach their devspaces
    terminal/
      terminal.go               who may open one, and the session record
      open.go                   the WebSocket: authorize, attach, audit open and close
    preview/
      preview.go                a port a devspace serves, and its address
      list.go  create.go  delete.go
      serve.go                  the proxy from the address to the guest
  template/
    template.go                 a stack: image, size, detect, build, run, port, health, cache
    list.go  get.go  create.go  update.go  delete.go

web/src/features/runtime/devspaces/
  index.ts                      what other features may use
  use-devspaces.ts              the generated calls and the event subscription
  ui/                           list, one devspace, create, terminal, previews
```

Nothing for this is added to `internal/api` or `internal/db`. The handlers and storage there for
today's workspaces are deleted in the last packet.

## The driver contracts

Added to `internal/hv` as optional interfaces, found the way `GuestManager` is today, so a driver
that cannot do one says so and the panel does not offer it.

| Contract | What it does | Needed by |
| --- | --- | --- |
| `GuestManager` (exists) | Images, create, delete | Everything |
| `GuestNetworkObserver` (exists) | The guest's addresses | Previews |
| `Executor` (new) | Run one command in a guest as a user, with input, bounded output and an exit code | Provisioning, AI client sign-in, chat turns, builds |
| `Terminal` (new) | Attach an interactive terminal as a user, with resize, until either side closes | The browser terminal |

`ports.Guests` is the panel-side port: the hosts a guest may be placed on, then create, start,
stop, delete, addresses, exec and attach on one of them. It replaces `ports.Machines`. It names no
provider.

## The devspace

**States.** `requested`, `provisioning`, `running`, `stopped`, `failed`, `removing`. A devspace is
a row first; the guest is made by a job. `failed` keeps the step that failed and its reason, and
offers retry and delete.

**Steps.** Provisioning is a job in the existing queue, so it survives a restart and runs once
across panel processes. Its steps are stored, not kept in memory, so any page can show them:

1. Check the organization's limits and reserve the size, in the transaction that saves the row.
2. Choose a host among those the organization may use (the placement policy).
3. Create the guest from the template's image and size.
4. Wait for an address.
5. Seed it: the person's SSH keys, the project folder, the template's files.
6. Start the template's run command.
7. Mark it running.

Each step first checks whether its result already exists, so a retried job continues where it
stopped.

**Limits.** `limits.Policy` gains what a devspace may use: how many, cores, memory and disk in
total. Create refuses over the limit before anything is made. The panel's default takes these from
settings; the enterprise module answers from the customer's plan. The host agent's own plan file
is not consulted.

**The user inside.** The template says which user a devspace runs as. The terminal, exec and
seeding read it from the template, so a terminal needs nothing else configured.

## Actions and routes

All generated from the files above. Members of the organization call them; managers change them.

| Action | Route |
| --- | --- |
| `shippin.runtime.devspace.list`, `create` | `GET`, `POST /v1/tenants/{tenantId}/runtime/devspaces` |
| `shippin.runtime.devspace.get`, `delete` | `GET`, `DELETE .../devspaces/{devspaceId}` |
| `shippin.runtime.devspace.start`, `stop` | `POST .../devspaces/{devspaceId}/start`, `/stop` |
| `shippin.runtime.devspace.preview.list`, `create`, `delete` | `.../devspaces/{devspaceId}/previews` |
| `shippin.runtime.template.list`, `get` | `GET /v1/tenants/{tenantId}/runtime/templates` |
| `shippin.runtime.template.create`, `update`, `delete` | `/v1/operator/runtime/templates` |

Two routes are transports, not actions, and are declared as such: the terminal WebSocket at
`.../devspaces/{devspaceId}/terminal`, and the preview proxy.

**Transports move into entity folders.** Today a transport is mounted by hand in `internal/api`.
For this engine to add nothing there, the generator learns a third kind of file beside actions and
reactions: a transport declared in its entity's folder, with what it carries, mounted from the
registry. This is a change to MCPL and is the first packet.

## What every read returns

```json
{
  "id": "…", "name": "shop", "state": "provisioning",
  "template": "react", "host": "…",
  "steps": [
    { "id": "guest", "label": "Creating the machine", "state": "done" },
    { "id": "address", "label": "Waiting for its address", "state": "running" }
  ],
  "actions": [
    { "id": "shippin.runtime.devspace.stop", "enabled": false, "why": "It is still being created." },
    { "id": "shippin.runtime.devspace.delete", "enabled": true }
  ],
  "terminal": { "available": false, "why": "It is still being created." }
}
```

The panel lists `actions` as buttons, disables one with its `why`, shows `steps` as they are, and
refetches when the event stream reports a change to this devspace. Labels are the server's. An AI
acting for the person reads the same answer and calls the same actions (MCPL-01).

## Previews

A preview is a port the devspace serves, given an address under a base domain set in the panel
(PUB-01). The panel itself proxies that address to the guest's address and port, after checking the
session, so a preview needs no separate edge proxy on the host and no host-side route state. The
installation supplies the wildcard name and certificate. The panel must be able to reach the
guests' network; where it cannot, the preview says so instead of failing silently.

## Storage

New tables, with the `shippin_` prefix: `shippin_devspaces`, `shippin_devspace_steps`,
`shippin_devspace_previews`, `shippin_templates`. Pre-release, so nothing is migrated from the old
`workspaces` table; it is dropped in the last packet.

## Proposed: one boundary per organization, containers inside

Raised by the owner on 2026-11-24: design for many organizations from the start, with the smallest
set of things a describe-build-preview-publish experience needs, extended by the editor and chat.
Proposed, not decided, and not checked on a rig.

**The boundary is the organization, not the devspace.** An organization gets one boundary on a
host, and its devspaces are containers inside it:

| Host | The organization's boundary | A devspace |
| --- | --- | --- |
| Incus | A restricted project with its own network and a storage quota | A container in that project |
| Proxmox | A virtual machine that runs Incus inside, with its own bridge | A container in that machine |

So a devspace is always an Incus container, reached through one implementation of `Executor` and
`Terminal`. Proxmox is used for what it does well, virtual machines, through the driver that makes
runner machines today. The question of a terminal on a Proxmox container goes away. The free tier
can share one pooled boundary machine on Proxmox, one project per organization inside it, so a free
organization does not cost a whole machine.

This adds one driver contract, `Boundary`: make, size and remove an organization's boundary, and
give the address guests inside it are reached on. `ports.Guests` then takes the organization.

**What a devspace needs for the default experience, and nothing more:**

| Surface | In the guest | Reached by |
| --- | --- | --- |
| Chat | The person's own AI command-line client, signed in by them | `Executor`, streamed |
| Preview | The template's run command on its port | The panel's proxy |
| Editor | A browser editor on one fixed port | The same proxy |
| Terminal | A Zellij session | `Terminal` |
| Publish | The template's build command | `Executor`, then the release is served |

Every surface is one of two driver contracts or the proxy, so the engine does not grow with the
number of surfaces. The image carries the user, Zellij and the editor; AI clients are installed from
configuration.

To check before it is decided: Incus nested in a Proxmox virtual machine on the local rig, and what
one idle boundary machine costs in memory.

## Where it stands, 2026-11-24

Existing code is being moved into the folders above before the engine underneath it changes.

| Moved | From | To | State |
| --- | --- | --- | --- |
| The devspace type and its storage | `internal/db/workspaces.go`, `ports.go`, `keys.go`, part of `operator.go` | `internal/runtime/devspace/devspace.go`, `ports.go` | Done. The table is still `workspaces`. |
| Firewall, domain and preview rules | `internal/api/guards.go` | `internal/runtime/devspace/rules.go` | Done |
| Templates: type, storage, rules, seed | `internal/db/templates.go`, `internal/api/template_check.go`, `templates.go` | `internal/runtime/template/` | Done |
| Template routes | Four hand-written staff routes | `shippin.runtime.template.list`, `get`, `create`, `update`, `delete` | Done. The panel calls the generated actions. |
| What a person may choose on create | `GET /v1/templates` | `shippin.runtime.devspace_option.get` | Done. The create page draws from it. |
| Devspace list, get, create, start, stop, delete | `internal/api/workspace*.go`, `job_workspace*.go` | Action files in `internal/runtime/devspace/` | Next |
| The terminal | `internal/api/terminal_*.go` | `internal/runtime/devspace/terminal/` | After that |
| Previews and the published site | `internal/api/workspace_previews.go`, `workspace_site.go` | `internal/runtime/devspace/preview/` | After that |

Still on the host agent until E1 to E3: making, starting, stopping and entering a devspace.

## Executing the move: the devspace lifecycle

Serves J1 step 1 (WS-01, MCPL-01). Each row is one commit that builds, passes CI and leaves one way
to do the thing: the hand-written route it replaces is removed in the same commit, and the panel
call is switched in the same commit. The host agent stays underneath, behind a port, until E1 to E3
replace what is behind the port.

### What the action framework lacks today

Read in `internal/mcpl/run.go` and `internal/api/mcpl.go`. Four things the lifecycle routes do that
a generated action cannot yet do:

| Gap | Today, hand-written | Added to the framework |
| --- | --- | --- |
| A support grant for one devspace | `canManageWS`, `sees` and `grantFor(id)` in `internal/api/grants.go`. The member gate only asks about the whole organization, so a holder of a one-devspace grant would be refused. | `Spec.Scoped`: the gate reads the item's id from the route and asks about that item. `Call.Sees(id)` filters a list. `Call.Grant` is the grant the call runs under, or empty for a member. The rules themselves do not change. |
| The audit entry of a grant's use | `auditUse` writes a separate entry before the work | The action's own audit entry carries the grant, break-glass and the devspace, in the same transaction as the change. One entry, not two. |
| An answer that is accepted, not finished | `202` with a `Location` to the job | An output that names its job is answered `202` with `Location`. |
| A repeated request | `a.idempotent` around the handler | The member mount wraps an action whose answer is accepted work, so a repeat with the same `Idempotency-Key` starts nothing new. Other changes are not recorded, because some answer with a secret shown once. |

### What the entity needs from the panel

| Need | Declared in | Provided today by | Replaced at |
| --- | --- | --- | --- |
| `ports.Guests`: what runs for an organization, make, apply an action, the current step, plan limits | `internal/ports` | The host agent client, in `internal/api/mcpl_services.go` | E2, by `internal/hv` |
| `devspace.Queue`: record a provision or an action as a durable job | The entity's package | `internal/services/jobs` | Stays |
| `devspace.Orgs`: the organization's name, plan, status and SSH address | The entity's package | `internal/db` (`TenantByID`), in `internal/api/mcpl_services.go` | When the organization's storage moves to `internal/tenancy/orgs` |
| The person's SSH keys | `internal/identity/ssh_key` | Its own storage | Stays |

### The commits

| # | Commit | Moves | Removes | Proof |
| --- | --- | --- | --- | --- |
| M1 | Framework: scoped member actions, accepted answers, idempotent accepted work | Nothing yet | Nothing | Done. `TestMemberGate` covers owner, member, grant for all, grant for one, break-glass, expired and pending, each for read, manage and decide. The audit entry and the `202` are first exercised by the M3 and M4 database tests, when an action uses them. |
| M2 | `ports.Guests` and `devspace.Orgs` | Reading what the hosts run goes behind the port. Making and applying join it in M4 and M5, with `devspace.Queue`. | The view built in `internal/api` | Done with M3 |
| M3 | `devspace.list`, `devspace.get` | `workspaces.go`, `workspace_view.go` into `list.go`, `get.go`, `view.go`. The answer gains `steps`: the list of provisioning steps the panel used to hold is now the server's. `actions` joins it in M4, when there are actions to name. | `GET .../workspaces`, `GET .../workspaces/{id}` | Done. Database test: a member sees their own and not another organization's, a one-devspace grant sees one, state is unknown when the hosts cannot be read. The panel list and the progress card read the action. Not yet seen in a browser. |
| M4 | `devspace.start`, `stop`, `restart`, `delete` | `workspace_action.go` into `start.go`, `stop.go`, `restart.go`, `delete.go`, `work.go`. Reads now carry `actions`, and the panel menu draws them. The job that carries the work out stays in `internal/api` until M4b. | `POST .../workspaces/{id}/actions/{action}` and the staff copy `POST /v1/operator/workspaces/{id}/{action}` | Pushed; first run of its database tests is in CI. Staff run the same action under their grant and still need the `workspaces.act` permission; a delete under a grant still needs the name typed. |
| M4b | The work itself | `job_workspace_action.go` into `apply.go`, registered from the entity | The handler in `internal/api` | The four existing job tests, moved with it |
| M5 | `devspace.create` | `workspace_create.go`, `workspace_spec.go`, `workspace_req.go`, `job_workspace.go` into `create.go`, `spec.go`, `provision.go` | `POST .../workspaces`. The secret-store folder, the rrflow attach and the ready notice become reactions to `shippin.runtime.devspace.created` in their own folders. | Create to running in the job test; replay with the same key returns the same devspace |
| M6 | `devspace.update` for firewall and domains | `workspace_access.go` into `update.go` | `PUT .../firewall`, `PUT .../domains` | Rule tests already moved; one action test |
| M7 | `devspace_fleet.get` for staff | `operator_workspaces.go` into `internal/runtime/devspace_fleet/get.go` | `GET /v1/operator/workspaces` | Staff matrix test |
| M8 | The panel draws from the answer | `use-workspaces.ts`, `use-devspace.ts`, `use-admin-actions.ts` call the actions. Buttons come from `actions`, progress from `steps`. | Hand-written `Workspace` types, `workspace-menu.tsx`'s own rules about which button shows | Panel tests; then as Customer in a browser on the review deploy: create, watch the steps, stop, start, delete |

After M8 the lifecycle has no file left in `internal/api`. The terminal follows (it needs packet
E0, transports in entity folders), then previews and the published site.

### What changes for someone calling the API

- Routes move from `/v1/tenants/{tenantId}/workspaces…` to `/v1/tenants/{tenantId}/runtime/devspaces…`.
  The old ones are removed, not kept beside the new.
- Start, stop, restart and delete are four actions, not one route with the verb in the path.
- A grant's use is audited as the action's own entry (`shippin.runtime.devspace.stop`), carrying the
  grant and the devspace, in the same transaction as the accepted work. A replayed request writes no
  second entry, because nothing happened.
- Staff no longer have a second route for start, stop and delete. The permission that route asked
  for, `workspaces.act`, is now asked of anyone running these actions under a support grant. Before,
  a grant holder without it could use the organization's own route.
- The answer to an accepted action is kept under its `Idempotency-Key` in the same transaction as
  the work, as before.

### What is not proved by these commits

Each is proved by tests in CI. None is seen in a browser until the review deploy runs, which needs
Docker on this machine. M8 is the commit that step 1 is looked at as Customer.

## Packets

Each closes only when its proof has been seen. Incus first on the local rig, then the same proof on
the Proxmox rig. A packet that would leave two ways to do one thing removes the old one in the same
change.

| Packet | Result | Proof |
| --- | --- | --- |
| E0. Transports in entity folders | A transport file in an entity folder is mounted from the registry | The events stream moves to its entity with no route changed; the conformance test passes |
| E1. Exec and terminal in the drivers | `Executor` and `Terminal` for Incus | On the rig: run a command as a user and read its exit code; attach, type, resize, close |
| E2. One guest port | `ports.Guests` replaces `ports.Machines`; runner machines use it | The runner tests pass unchanged; a runner machine is made and removed on the rig |
| E3. The devspace entity | Create, get, list, start, stop, delete, with stored steps and the limits check | On the rig through the API: create to running, stop, start, delete; a second person is refused; over the limit is refused before a guest exists; a panel restart mid-provision continues |
| E4. The terminal | The browser terminal on a devspace | In a browser on the review deploy: type, resize, drop the connection and return to the same session; another person is refused |
| E5. The panel | The devspaces feature drawn from `steps` and `actions` only | As Customer in a browser: create, watch the steps, open the terminal, stop, delete, with no rule in the page |
| E6. Templates and previews | React, Svelte, Go and Rust as stack data; a preview address | Each of the four starts from its template, builds, runs and shows at its preview address |
| E7. The old path goes | Today's workspace handlers, storage and the host agent client leave the panel | The build has no import of `hostclient` or `hostd`; every route they served is gone or re-authored |
| E8. Proxmox | The same contracts for Proxmox containers | E3, E4 and E6 repeated on the Proxmox rig |

E1 to E5 are step 1 of the current target. E6 is step 2.

## What E7 takes with it

These features reach a devspace through the host agent today. Each is re-authored on `Executor`
when its own step comes, and is absent from the panel between E7 and then:

| Feature | Re-authored as | When |
| --- | --- | --- |
| AI client sign-in, status and sign-out | Exec in the person's devspace | Their own AI (step 6 of the coverage plan) |
| Chat turns | Exec in the devspace | The same |
| One-click site publish | A release built per stack and served at an address | Publish (step 5) |
| Git import and push | Exec in the devspace | Templates (E6) |
| SSH relay for the bridge | `Terminal` and a forwarded port | Remote mode (step 11) |
| Per-customer storage, network and VM boundary | Enterprise module | With the separation |

## Decisions needed from the owner

- **E7's gap.** Whether the features in the table above may be absent from the panel between E7 and
  their own step, or whether E7 waits until each is re-authored. This plan assumes they may be
  absent, because keeping the old path alive beside the new one is two engines.
- **Proxmox containers or VMs.** A Proxmox devspace as a container matches Incus and the free
  tier's cost. If Proxmox's API cannot give an interactive terminal on a container, the choice is a
  VM with a guest agent, or a small agent in the image.
