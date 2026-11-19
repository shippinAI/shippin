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

## Extension points (to build)

All of these are registered by the module at start-up, in `internal/ext`. The panel works the same with no module registered.

- **Start-up.** The console's wiring is one function, `app.Run`. The panel's `cmd/console` calls it, and so does the enterprise binary, after importing its module packages.
- **Actions.** The module keeps its MCPL actions in its own `internal/<domain>/<entity>/<verb>.go` files. The generator scans the module the same way it scans the panel and writes the module's own registry. The panel merges it into its routes, audit, events, the OpenAPI document and the typed panel calls. An action keeps its ID and route when it moves between the two.
- **Settings sections, permissions and job kinds.** The module adds sections to the Settings page, permissions to the catalog (staff presets and grants work unchanged), and kinds to the durable job queue.
- **Database.** The module brings its own migrations. They run after the panel's, in the same transaction rules, lock and migration table.
- **Policies.** Each place where the panel needs a business rule asks a policy, and the panel's default is the self-hosted answer:

| Policy | Asked when | Panel default |
| --- | --- | --- |
| Caller | A person signs in or opens an organization: is their sign-in strong enough, and which other organizations may they see? | No extra rule, no extra organizations |
| Organization lifecycle | An organization is created, suspended or removed, and before work starts in it: is it ready? | Ready at once, nothing to provision |
| Limits | Work starts: how many at once, how large? | The limits in the panel's settings |
| Placement | Work is placed: which hosts may this organization use? | Every host in the installation |

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
