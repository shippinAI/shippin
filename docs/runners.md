# Build runners

How Shippin runs its customers' GitHub Actions jobs (RUN-01 in [What Shippin is](objective.md)).
Status and order are in the [roadmap](roadmap.md); this page is the design.

## What a customer sees

1. Their organization connects GitHub with the Shippin GitHub App, as it already does to import
   projects.
2. They assign repositories to a workspace or project.
3. A workflow that says `runs-on: shippin-<size>` (for example `shippin-2x`) runs on a fresh VM
   used for that job alone. It is removed when the job ends.
4. Their workspace shows the queue, running jobs and history with logs. Their plan sets the sizes
   they may use, how many jobs run at once and how many minutes are included.

## What it is built on

Nothing here is a second process or a second GitHub integration.

| Need | Existing piece |
| --- | --- |
| GitHub sign-in, tokens, webhooks | `internal/gh`: the GitHub App, narrowed installation tokens cached per installation, signature checks. `github_installations` maps each installation to one organization; `github_deliveries` drops repeated deliveries. `POST /v1/github/webhook` is the one route GitHub calls. |
| Runner machines | `internal/hv`: `GuestManager.CreateGuest` and `DeleteGuest` on Incus and Proxmox now, public clouds (HYB-01) and customers' own servers (BYO-01) later. Placement follows [VM-first tenants](reference/vm-first-tenants.md). |
| Doing the work reliably | `internal/jobs`: the durable queue, claimed under a lease, so a runner is started and removed once, across restarts. |
| Plan limits | The plan matrix: a `runners` catalog service, whose cell on each plan holds its limits (`sizes`, `concurrent`, `minutes`). `concurrent` 0 runs nothing; `minutes` 0 sets no monthly cap. |
| Actions, events, audit | MCPL: every operation is an action file; events drive reactions; staff changes need a reason. |

## Entities

| Entity | Who | Actions |
| --- | --- | --- |
| `services/runner_size` | staff | list, get, create, update, delete: name (the `shippin-<name>` label), CPU, memory, disk, image, active |
| `integrations/github_repository` | member (manage) | list (the organization's installations' repositories), update (assign to a workspace or project, or allow runners), delete (unassign) |
| `runtime/runner_job` | member | list, get (with logs), cancel |
| `runtime/runner` | staff | list (every runner VM, its organization, host and age), delete (remove a stuck one, with a reason) |

## A job's life

1. GitHub sends `workflow_job` with action `queued`. The webhook route checks the signature,
   drops a repeated delivery, and finds the organization from the installation and the workspace
   or project from the repository's assignment. A job for an unassigned repository, an unknown
   label, or a size the plan does not include is recorded as refused, with the reason, and gets no
   runner.
2. A `runner.provision` job starts it when the organization is under its concurrency limit;
   otherwise it waits in order.
3. It asks GitHub for a just-in-time runner configuration (`generate-jitconfig`) for that
   repository with the job's labels. The configuration is single-use; it goes to the VM's first
   boot and is never stored or logged.
4. It creates the VM from the size's image, on capacity the organization may use, named after the
   job.
5. The runner takes exactly one job and exits. `workflow_job` `completed` records the result and
   the minutes, and a `runner.remove` job deletes the VM. A VM older than the job time limit is
   removed regardless.

## Isolation

- One VM per job, per organization. Runners are never pooled, reused or shared.
- The VM holds only what that job's workflow brings. No Shippin credential is on it.
- A repository is used only after a member assigns it, so a public repository's fork pull requests
  run only where its owners chose.
- Tokens are installation tokens narrowed to the one repository and the permission the call needs.

## Images

A runner image is a provider setting: an Incus or Proxmox image with the GitHub Actions runner
installed, built from a pinned release whose checksum is verified, with its provenance recorded.
Customers' own runners (catalog entry, CAT-01) install the same pinned release on their machine and
are labelled customer-owned.

## What the GitHub App needs

The panel's GitHub App (configured as for project import) must also:
- subscribe to the **Workflow job** event, with **Actions: read**, for jobs to reach the panel;
- have **Administration: read and write** on repositories, for just-in-time runner configurations
  (stage 2).

## Order

1. **Done 2026-11-15.** Sizes (`services/runner_size`), repository assignment
   (`integrations/github_repository`), and jobs (`runtime/runner_job`): a `queued` job is recorded
   against its organization, waiting or refused with the reason, then moved on by `in_progress` and
   `completed`. Database tests with a fake GitHub cover two organizations side by side; the webhook
   route is tested with signed and unsigned deliveries.
2. Dispatch: JIT configuration and a VM per job on Incus in the local rig, then removal.
3. The panel: the workspace's runner pages and staff's runner list, on generated calls.
4. Proxmox, then public clouds and customers' own servers as they land.
5. The catalog entry for a customer's own runner; usage records for metering (stage 14).

What came before: pull requests #18 and #20 proposed a separate `shippin-run` process with its own
hand-written MCP, file storage and no tenant scoping, and a panel page proxying a gateway in another
repository. Its tested helpers (the `runs-on` rewrite, log redaction, the health rule) are reviewed
and brought in where a step above needs them.
