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

### As built (stage 2)

Code: `internal/runtime/runner` (`dispatch.go`, the staff entity, the removal reaction), with the
host ports `ports.GitHub.JITConfig` and `ports.Machines` provided by the panel
(`internal/api/runners.go`).

- **Machines.** `shippin_runner_machines` (migration 075) holds one row per runner job, ever
  (`runner_job_id` is unique), so a job is never started twice. The row carries the organization in
  its reference to the job, the host, the hypervisor's id (`guest`, which is also the runner's name
  on GitHub), the attempt and the state: `starting`, `running`, `removing`, `removed`, `failed`.
  The runner configuration is never in it.
- **Starting.** `runner.provision` (target: the runner job) is queued when a job is recorded as
  waiting, and by the sweep. Under a per-organization lock it starts the job only if the
  organization has fewer machines not yet removed than its plan's `svc.runners.concurrent`
  (`catalog.EffectiveFor`; a plan without runners, or 0, runs nothing) and no earlier waiting job of
  the organization would be passed over. It places the machine on the least loaded host of the
  configured fleet, records it, then registers the runner, makes the VM with cloud-init user data
  and records it running. An attempt holds the machine for 15 minutes, so a second run of the same
  job finds it busy; a failed attempt is carried on by the next with a new runner name, after
  deleting what it left; the last failed attempt leaves the machine `failed` and the job says
  Shippin could not start a runner.
- **Fairness.** The sweep queues starts for each organization's room only, taking one job from each
  organization in turn, so one organization's backlog never queues ahead of another's first job.
- **Removal.** `workflow_job` `completed` (or `cancelled`) marks for removal the machine whose
  runner ran the job (by runner name) and the machine started for the job if its runner never took
  another, then queues `runner.remove` (target: the machine). GitHub may give a waiting runner any
  queued job of the same repository and labels, which is why `in_progress` records the job each
  runner took. A failed delete leaves the machine `failed` with the error; the sweep tries again.
  After a removal the organization's next waiting jobs are queued.
- **Sweep.** The reconciler runs it every `jobs.reconcile_seconds`: machines older than
  `runners.job_limit_minutes` are marked for removal, removals that failed or were cut short are
  queued again, and waiting jobs are queued where there is room.
- **Staff.** `runtime/runner` list (`infra.read`) and delete (`infra.guests.manage`, with a reason);
  the reaction `remove_machine` deletes it from its host once the change is committed. Machine
  creates and deletes are in the host's change history as "Build runners".
- **Configuration.** `runners` in the panel's configuration: `enabled` (needs `jobs.enabled`),
  `fleet` (the fleet whose hosts run runner machines; staff add hosts to it in the panel), `storage`
  (the pool for the root disk sized from the runner size; empty keeps the profile's) and
  `job_limit_minutes`. Which organizations get runners, which sizes and how many at once stay plan
  settings.
- **Providers.** Incus passes the user data as `cloud-init.user-data` and sizes the root disk when
  a pool is set. Proxmox refuses user data as unsupported for now; a bare server has no guest API.

Known limits: a job canceled while the machine started for it has just been given another job,
before GitHub says so, can take that machine with it; a host that stays down fails a job's start
after its attempts and the job then waits without a runner until GitHub gives up on it; removing a
machine does not yet delete an unused runner registration on GitHub (GitHub drops single-use
runners that never connect).

## Isolation

- One VM per job, per organization. Runners are never pooled, reused or shared.
- The VM holds only what that job's workflow brings. No Shippin credential is on it.
- A repository is used only after a member assigns it, so a public repository's fork pull requests
  run only where its owners chose.
- Tokens are installation tokens narrowed to the one repository and the permission the call needs.

## Images

A runner image is a provider setting: an Incus or Proxmox image with the GitHub Actions runner
installed, built from a pinned release whose checksum is verified, with its provenance recorded.
Dispatch expects, in a VM image with cloud-init: the runner unpacked in `/opt/actions-runner`, a
user `runner` that owns it, and `runuser`. The first boot writes the configuration root-only to
`/run/shippin-runner/config`, runs `./run.sh --jitconfig` once as `runner`, removes the
configuration and powers off. No such image is built yet.
Customers' own runners (catalog entry, CAT-01) install the same pinned release on their machine and
are labelled customer-owned.

## What the GitHub App needs

The panel's GitHub App (configured as for project import) must also:
- subscribe to the **Workflow job** event, with **Actions: read**, for jobs to reach the panel;
- have **Administration: read and write** on repositories, for just-in-time runner configurations
  (stage 2). Each configuration is asked for with an installation token narrowed to the job's one
  repository and `administration: write`, and the runner joins the repository's default runner
  group (id 1).

## Order

1. **Done 2026-11-15.** Sizes (`services/runner_size`), repository assignment
   (`integrations/github_repository`), and jobs (`runtime/runner_job`): a `queued` job is recorded
   against its organization, waiting or refused with the reason, then moved on by `in_progress` and
   `completed`. Database tests with a fake GitHub cover two organizations side by side; the webhook
   route is tested with signed and unsigned deliveries.
2. **Built 2026-11-15, on a branch.** Dispatch: JIT configuration and a VM per job, then removal
   (As built, above). Database tests with a fake GitHub and fake hosts cover arrival order, the
   limit per organization, fairness between two organizations, no double start, removal on
   completion, cancellation and timeout, failures, and organizations kept apart. On the local Incus
   rig a cloud-init container (the rig cannot run VMs) was made with the dispatch user data, ran it
   and powered off, and was removed. Not yet proved: a VM, a runner image, and a real GitHub job.
3. The panel: the workspace's runner pages and staff's runner list, on generated calls.
4. Proxmox, then public clouds and customers' own servers as they land.
5. The catalog entry for a customer's own runner; usage records for metering (stage 14).

What came before: pull requests #18 and #20 proposed a separate `shippin-run` process with its own
hand-written MCP, file storage and no tenant scoping, and a panel page proxying a gateway in another
repository. Its tested helpers (the `runs-on` rewrite, log redaction, the health rule) are reviewed
and brought in where a step above needs them.
