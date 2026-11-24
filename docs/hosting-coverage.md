# Hosting coverage plan

What it takes for Shippin to cover what WHMCS and cPanel cover, for modern stacks, with
one-click deployment and a development space that defaults to a describe-build-preview-publish
experience on the person's own AI subscriptions.

This page is a plan. [What Shippin is](objective.md) stays the contract and owns the requirement
IDs; [the roadmap](roadmap.md) owns the order of work and what is proved. Where this page proposes
something the contract does not have yet, it says **proposed**, and it is not a requirement until
the contract takes it.

States below were read in the code on 2026-11-24. None was checked on the review deploy. **Works**
means the code path exists end to end; **partial** means some of it; **absent** means nothing.

## The shape

Three layers, and each maps to one of the products being replaced:

| Layer | Replaces | Edition | Who uses it |
| --- | --- | --- | --- |
| The account panel: one project's apps, addresses, databases, files, secrets, logs, backups | cPanel | Panel | The person who owns the project |
| The provider's business: products, orders, customers, usage, invoices, support | WHMCS | Enterprise module | The hosting company |
| The development space: where the project is built, in whichever way the person works | Neither had one | Panel, hosted by the module | The person building |

The first two are what a hosting company expects. The third is why a customer picks Shippin over
a host that only runs finished code.

Two rules hold across all three:

- **One executor.** A click in the catalog, a Git push, a publish from the development space and an
  AI acting for the person all produce the same versioned plan and run through the same deployment
  executor (DEP-01). There is no second path that only one of them uses.
- **Provider-neutral.** Everything below runs on Incus and on Proxmox through `internal/hv`. A
  capability is not done while it works on one of them only.

## The account panel (cPanel's ground)

| cPanel gives | Shippin's equivalent | Requirement | State |
| --- | --- | --- | --- |
| Upload a site | Deploy from a template, a Git repository or the development space, built per stack | DEP-02 | Partial: publish copies the development space and serves its dev server. No build step, no deploy from Git. |
| Softaculous one-click apps | The service catalog: entries are data, each with versions, inputs and a health check | CAT-01, SVC-01 | Partial: catalog and plans exist; one executor path, Incus only, never run on a real host. No entries seeded for PostgreSQL or ClickHouse. |
| Domains and subdomains | An address under a base domain; custom domains with ownership proof | PUB-01, PUB-02 | Partial: platform subdomain works and names are owned by one workspace. Custom domains have no ownership proof. |
| DNS zone editor | The exact records to set, checked; optional scoped automation | PUB-02, NET-01 | Partial: Cloudflare and Tailscale inventory is read-only. |
| SSL certificates | Issued and renewed automatically, verified before an address is switched | PUB-01 | Not determined in the code read; the edge proxy routes exist. |
| MySQL and phpMyAdmin | Managed PostgreSQL first, with a database page in the panel | CAT-01, SVC-03 | Partial: a shared starter database and a panel page. One managed kind, and it is not a database engine. |
| File manager, FTP | The editor in the development space; SFTP through the bridge | IDE-01, AI-03 | Partial: SSH to one workspace works; no file page outside the editor. |
| Environment and secrets | Per-project secrets from the secret store, injected at run | DEP-01, CFG-01 | Partial: resolved for Incus deployments; not for a development space run. |
| Cron jobs | Scheduled jobs and background workers as deployment kinds | **Proposed** | Absent. |
| Logs and metrics | Build, run and access logs; CPU, memory, disk and network over time | PUB-03, OBS-01 | Partial: host and guest samples reach ClickHouse; no per-app logs page. |
| Backups | Scheduled, off-machine, with a tested restore | SVC-03, REL-02 | Stub: a backup is recorded and never taken; restore refuses. |
| Email accounts | Mailboxes and domains as a managed service | **Proposed** (plans name mail limits) | Absent: the panel sends notices only. |
| PHP version selector | A runtime version per stack, from the template | DEP-02 | Absent: templates do not carry a version. |
| Terminal | A real shell that reconnects, with the plain-language layer | TERM-01 | Works for the shell; the plain-language layer is absent. |

### Stacks

A stack is data: how to detect it, build it, cache it, run it, check it and which port it serves.
Templates and deployments both read it, so a project started from a template deploys with no
further questions.

| Stack | Build | Runs as | First |
| --- | --- | --- | --- |
| React (Vite) | `npm ci`, `vite build` | Static files | Yes (J1) |
| Svelte (SvelteKit) | `npm ci`, `vite build` | Node server or static, by adapter | Yes (J1) |
| Go | `go build` | One binary | Yes (J1) |
| Rust | `cargo build --release` | One binary | Yes (J1) |
| Vue, Astro, Next.js, Nuxt | Their own | Static or Node server | After J1 |
| Python, any Dockerfile | Their own | Process or image | After J1 |

Today there are four templates (node-vite, python, go, blank), each a single start command with no
build. Svelte and Rust are missing.

## The provider's business (WHMCS's ground)

All of this is the enterprise module's. The panel supplies the extension points; the module holds
the business.

| WHMCS gives | Shippin's equivalent | Requirement | State |
| --- | --- | --- | --- |
| Products and pricing | Plans and a plan-by-service matrix, edited in the panel; prices as settings | PLAN-01, SUB-01 | Works for editing. Plans do not yet shape anything on a host. |
| Order form and client sign-up | Sign-up that creates the account, the organization and its plan in one step | LAUNCH-01, PRV-01 | Partial: invitation, waitlist and open modes; an invitation does not yet create the organization on a plan. |
| Provisioning modules | An order becomes resources through the same executor and the module's lifecycle policy | PRV-01, DEP-01 | Stub: provisioning intents are recorded and nothing applies them. |
| Suspend, unsuspend, terminate | One lifecycle with those states, applied to everything the customer runs | **Proposed** (IAM-02 covers access only) | Partial: suspend stops development spaces only; deployments and published sites keep running. |
| Upgrade and downgrade | Changing the plan re-applies limits everywhere | SUB-01 | Absent: a plan change updates one row. |
| Quotas | Limits read at the point of use, in the same transaction as the work | TEN-01 for customers | Partial: build runners only. |
| Usage and billing | A stored usage record per customer, then invoices from it | BILL-01 | Partial: a live rollup with rates; no stored record. Invoices absent. |
| Payment gateways | An integration that records payments and drives the lifecycle | BILL-01 | Absent. Not needed for the friends-and-family launch. |
| Support tickets | Tickets tied to the customer, their coverage tier and the affected resource | COV-01 | Absent. |
| Client area | The same panel, as a customer | UI-01 | Works as a shell; depends on everything above. |
| Staff roles and audit | Scoped staff roles, support grants, an audit log with reasons | IAM-01, IAM-02, GOV-01 | Partial: several permission models overlap. |
| Reseller and white-label | Branding per installation; resale as a later decision | **Proposed** | Absent. |
| Third-party modules | Provider drivers, catalog entries and payment or registrar integrations others can write | **Proposed** | Absent: extensions are Go modules under the panel's own import path. |
| Domain registration | A registrar integration | **Proposed** | Absent. |

What the extension seam needs before the business can move out of this repository:

1. Limits for every kind of work, not build runners alone.
2. Suspend and resume in the organization lifecycle.
3. A declared public route a module can receive a payment or registrar webhook on.
4. A stored usage record the module reads and the panel writes.
5. One source for plan limits. Today the database, the host agent's file and a config default each
   hold them, and the host agent refuses a plan it was not told about.

## The development space

One space, several ways to work in it. The space, its files, its runs and its previews are the
same in every mode; a mode is only which surface is in front (IDE-01). A person picks a default
and switches at any time. Nothing is hidden in any mode.

| Mode | For | What is in front | State |
| --- | --- | --- | --- |
| **Build** (the default) | Describing what you want and watching it appear | A chat, the live preview beside it, and Publish. The person's own AI does the work; the files it changed and the commands it ran are shown as they happen (GUIDE-01). | Partial: chat exists for two AI clients; no preview beside it, no one screen. |
| **Code** | Editing directly | The editor in the browser, the terminal below, the preview beside | Partial: an editor route exists per workspace. |
| **Terminal** | Working in a shell, with AI command-line clients | The Zellij session and the plain-language layer | Works for the session. |
| **Remote** | Working from your own machine | VS Code Remote, a desktop AI client or an MCP client, connected through the bridge | Partial: SSH works; the bridge is not in any release and setup is by hand. |
| **Dispatch** | Handing off a task and reviewing the result | A queue of runs with their logs, changed files and result | Absent (AI-02). |
| **Import** | Bringing a project in | From a Git repository, or from another builder | Partial: GitHub import exists. |

Their own subscriptions (AI-01, DEV-01):

- The person signs in to each AI client with their own account, inside their own space. Shippin
  never holds the credential. Today a pasted token is stored in the secret store and read back at
  start; that path has to go or become an explicit, separate API-key mode (AI-02).
- One sign-in per person, not per space. Today each new space needs a fresh sign-in.
- Clients are configuration, not code: each entry says how to install, sign in, check and sign out.
  Claude Code and Codex are wired; Gemini is not.
- Connecting from a person's own machine is one command that signs the device in and writes the
  editor's configuration, on Windows, macOS and Linux.

## Order

This is the roadmap's order with the gaps above placed in it. Each step closes only when it is
seen working on the review deploy, on both hypervisors where it touches one.

| Step | Result someone can see | Covers |
| --- | --- | --- |
| 1 | A development space is created on a host added in the panel, through `internal/hv`, with its terminal ([the engine plan](reference/devspace-engine.md)) | WS-01, TERM-01; ends the separate host-agent path for spaces |
| 2 | Stacks as data. A React, Svelte, Go or Rust project starts from a template, builds, runs and previews | DEP-02 |
| 3 | Sign-up lands a person in their own space on a plan | LAUNCH-01, DEV-01 |
| 4 | The project's secrets are injected when it runs | DEP-01 |
| 5 | Publish builds a release and serves it at an address; sites are rows, with history and rollback | PUB-01, PUB-03 |
| 6 | Build mode: chat, preview and publish on one screen, on the person's own AI | IDE-01, AI-01 |
| 7 | One-click services: PostgreSQL first, on both hypervisors, with backup and restore | CAT-01, SVC-03 |
| 8 | Deploy from Git, custom domains with ownership proof, per-app logs | DEP-02, PUB-02, PUB-03 |
| 9 | Plans are the one source of limits, applied on create and on change; suspend covers everything | SUB-01, TEN-01 |
| 10 | The seam is widened, then the business code moves to the enterprise module | OPEN-01 |
| 11 | Remote mode in one command; more AI clients; Dispatch | AI-02, AI-03 |
| 12 | Stored usage, then invoices and a payment integration; support tickets | BILL-01, COV-01 |
| 13 | Mail, scheduled jobs, a registrar, public clouds, third-party modules | Proposed items, HYB-01 |

Steps 1 to 5 are the current target, a free-tier developer ships a project. Steps 6 to 8 make the
account panel complete for one person. Steps 9 to 12 make it a business. Step 13 is what remains
of the two products being replaced.

## Decisions this plan needs

- Whether cron and workers, mail, a registrar, reseller and third-party modules enter the contract,
  and with which requirement IDs.
- Whether a pasted AI token is removed or kept as an explicit API-key mode.
- Whether third parties may write modules, which needs a small public package outside `internal/`
  for the driver and module contracts.
