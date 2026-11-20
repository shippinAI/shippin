# Editions and licensing

Shippin comes in two editions (OPEN-01). This page says what each one contains, how they are licensed, and how the enterprise edition is built on top of the panel.

## Two editions

**The panel** is this public repository. It is a self-hosted control panel, in the spirit of Coolify:
- one installation, run by its owner and their team;
- their own Incus and Proxmox hosts, added in the panel;
- the apps, services, virtual machines and development workspaces on those hosts.

It installs on a host the owner already has, and it manages that host, and others, from then on.

**The enterprise edition** adds what is needed to run a multi-tenant hosted cloud and sell it:
- customers;
- plans, coverage and quotas;
- sign-up;
- isolation between customers;
- the hosted development space.

It lives in the private repository `shippinAI/shippin-enterprise`, as a module the panel loads. It is not a fork.

| Area | The panel (public) | Enterprise module |
| --- | --- | --- |
| People | The owner, their team, staff roles, local accounts, passkeys, OIDC sign-in, invitations to the team | Customers as organizations, sign-up (open, waitlist, invitation onto a plan), acquisition |
| Organizations | The installation's organization and any the owner adds; workspaces and projects in them | Customer organizations with a plan and coverage |
| Infrastructure | Incus and Proxmox hosts, guests, storage, networks, fleets, metrics, the host agent for a server the owner brings | Hosts assigned to customers, customer-brought servers and cloud accounts, placement between customers |
| Workspaces | Development workspaces as containers on the owner's hosts, opened in the browser terminal through the hypervisor's API | The hosted development space for customers: per-customer isolation with encrypted storage, private networks and a VM boundary (hostd) |
| Services | The catalog, deployments, managed services, build runners, publishing, domains | Services and runners limited by a customer's plan; customer starter databases; hosted Vaultix |
| Access | Staff roles, the audit log, governance and review | Access to customers: grants, support access, break-glass, staff mode, per-coverage sign-in rules |
| Business | None | Plans, coverage tiers, quotas, metering, billing |

The rule for anything new: if an owner running their own installation needs it, it belongs to the panel (SOLO-01). If it only matters when an installation sells to customers, it belongs to the enterprise module.

## The license

This repository is licensed under the [Functional Source License 1.1, Apache 2.0 future license](../LICENSE.md) (FSL-1.1-ALv2). In plain terms, and the license text is what counts:

- Anyone may use, copy, modify and self-host it, for any purpose except a competing use.
- A competing use is making it available as a commercial product or service that substitutes for Shippin, such as selling hosting run on it. That needs a commercial license from shippinAI.
- Each version also becomes available under Apache 2.0 two years after it is released.

The enterprise module is commercial software, used under an agreement with shippinAI.

Outside contributions need a contributor agreement first, so that contributed code can also ship in the enterprise edition. None is set up yet. Until it is, only shippinAI's own commits are accepted.

## How the module is built

| Part | Where | License |
| --- | --- | --- |
| The panel, the control plane, the host agent for owner servers, every self-hosted feature | This repository, Go module `shippin.cloud/console` | FSL-1.1-ALv2 |
| Enterprise features | `shippinAI/shippin-enterprise`, Go module `shippin.cloud/console/enterprise`, pinning this repository at a commit | Commercial |
| The enterprise build | An image built from the pinned panel plus the module | Commercial |

The module's path sits under the panel's, so Go lets it use the panel's internal packages. It never copies or patches panel code. When it needs something the panel does not offer, the panel gains an extension point here, in public, and the module uses it. So a fix in the panel reaches both editions in the next build.

## Extension points

All of these are registered by the module at start-up, in `internal/ext`. The panel works the same with no module registered.

Built:

- **Start-up.** The console's wiring is one function, `app.Main` in `internal/app`. The panel's `cmd/console` calls it. The enterprise binary calls it too, after importing its module packages, whose `init` functions call `ext.Register`.
- **Actions, reactions, services and settings sections.**
  - The module keeps its MCPL actions, reactions, entity services and `settings.go` sections in its own `internal/<domain>/<entity>/` folders.
  - `cmd/mcplgen -module shippin.cloud/console/enterprise -pkg <name> -out <path>` scans the module the same way it scans the panel and writes the module's own registry, which the module passes to `ext.Register`.
  - The panel reads every action, reaction, service and section through `ext`, so the module's are routed, audited, published as events, served over MCP and shown on the Settings page like the panel's.
  - An action keeps its ID and route when it moves between the two.
  - A name declared twice, by the panel and a module or by two modules, stops the binary at start.
  - The OpenAPI document and the typed panel calls for the enterprise build come from the merged list: `contract.Files(ext.Actions())`.
- **Permissions.**
  - A module's permissions join the panel's catalog, so its staff routes can name them, roles can hold them and the Team page lists them.
  - Direct grants and presets work unchanged. A permission's `Presets` are the preset roles that hold it on a fresh database; an existing installation's roles gain it through the module's migrations.
  - `app.Main` loads them once, with `api.LoadModules`, before the API is built. A key the panel already uses stops the binary.
- **Job kinds.** A module's kinds of durable job run on the panel's queue, with the panel's database and the services its actions use (`ext.Runtime`). A kind the panel already handles stops the queue from starting.

- **Database.** A module brings its own migration files (`ext.Module.Migrations`). They run after the panel's, in name order, under the same lock and transaction rules, and are recorded in the same table as `module:<name>/<file>`, so a module's numbering never meets the panel's. While a panel migration is held back (`migrations.hold`), no module migration runs.

To build:

- **Policies.** Each place where the panel needs a business rule asks a policy, and the panel's default is the self-hosted answer:

| Policy | Asked when | Panel default |
| --- | --- | --- |
| Caller | A person signs in or opens an organization: is their sign-in strong enough, and which other organizations may they see? | No extra rule, no extra organizations |
| Organization lifecycle | An organization is created or removed, and before work starts in it: is it ready? | Ready once active, nothing to prepare |
| Limits | Work starts: how many at once, how large? | The limits in the panel's settings |
| Placement | Work is placed: which hosts may this organization use? | Every host in the installation |

  Built so far:
  - **Caller** (`ext.CallerPolicy`, set with `ext.Module.Caller`). The caller policy hands the panel each organization's label and sign-in rule (`ext.OrgRule`), and the organizations a staff member reaches without membership, as `ext.Access`. Access is re-asked on every request, and its use is audited, as break-glass when it says so.
  - **Organization lifecycle** (`ext.OrgLifecycle`, set with `ext.Module.Lifecycle`). `Prepare` runs when an organization is created and on every retry, so it must be safe to repeat; the address it returns is recorded with the organization. `Ready` decides whether a workspace may be created, and its reason is shown when not. `Remove` runs when staff delete the organization, and names anything it kept. The panel keeps the organization's status throughout.

  Until the enterprise code moves out (roadmap S3), the policy the panel uses when no module sets one is today's behaviour, from code still in this repository: coverage tiers' sign-in rules, support and break-glass grants, and the per-customer silo that shippin-hostd builds and tears down. The defaults in the table take over when that code moves.

- **Panel pages.** Module pages sit in `web/src/modules/<name>/` in the enterprise build. Each registers its routes, navigation entries and cards. They load lazily, so the panel's own build carries none of them.

## The license key (to build)

- An enterprise build runs the module's features only with a valid key: a small file signed with shippinAI's private key. It names the customer, the edition, the features, the number of hosts or customers allowed, and an expiry date.
- The panel checks the signature offline against the public key built into the enterprise image, so an unmanaged installation never has to call home.
- Settings has a page to upload the key. It shows what it covers and when it expires, and warns 30 days before it expires.
- When a key expires, the enterprise features turn read-only for a grace period, then off. Self-hosted features, customer data and running workloads are never affected.
- The panel's own build has no key check at all, because it contains no enterprise code.

## Managed and unmanaged

Both run the same enterprise image:

- **Managed:** shippinAI runs the installation for the customer.
- **Unmanaged:** the customer downloads the image and runs it on their own machines with their key.

## Where the split stands

Today the multi-tenant code still lives in this repository, and the panel's own team works inside a customer organization with a plan. The [roadmap](roadmap.md#separation-first) moves it out, one area at a time, into the enterprise repository, and then adds a check that keeps it out.

## Not decided here

- Whether a key also limits usage (hosts, customers) or only enables features.
