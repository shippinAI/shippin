# Fleet operations

What an operator running Shippin against real Incus and Proxmox servers needs from the panel, stated as requirements a change can be checked against. The status of each is what the code does today.

## Terms

- **Host (node):** one hypervisor API the panel can reach. It is always exactly one provider.
- **Provider:** a kind of hypervisor, such as Incus or Proxmox VE. A driver in `internal/hv` implements it.
- **Guest:** a virtual machine or container on a host.
- **Native cluster:** a cluster the hypervisor forms itself. Proxmox uses corosync and pmxcfs. Incus uses its own raft database. They own quorum, shared state and live migration.
- **Fleet:** a group of hosts in the panel, for seeing them together. It is a management-plane idea and changes nothing on the hosts.

## Requirements

### FO-1 Anyone can deploy and develop it

A person with Docker gets a working panel from a clone and one command, with no configuration. A real site changes only a file it keeps. Nothing about one installation is in the repository.

**Done when:** CI clones, runs `docker compose up`, creates the first owner and signs in. CI fails on a site name, address or product domain in shipped code.

**Status:** Done.

### FO-2 A new build is visible

The panel shows which commit it is running and says when a newer one is on the configured branch, with what changed and the exact command for how this panel was deployed. The source is configuration, so a fork follows its own repository.

**Done when:** an operator sees the notice without leaving the panel. It never says "up to date" when the check failed or has not finished.

**Status:** Done. Updating from a button is not built: it needs a privileged updater outside the panel process, a health check after the update, and an automatic rollback. Until then the panel shows the command.

### FO-3 Each host says what it is

Every host shows its provider and the version its API reports, read live.

**Done when:** a host's version and provider are visible on the overview and on its tab. A host that does not answer stays listed with the reason and is never shown as online.

**Status:** Done, and run against one real node of each: provider, version, reachability and guest counts on the overview, and release, kernel, CPU, memory, uptime and native cluster members on the tab. The tab opens on a host that answers, so a dead one does not hide the others.

### FO-4 Lifecycle operations are reliable and visible

Starting or stopping a guest is not finished when the request is accepted. It is finished when the hypervisor's task has ended and a fresh read shows the state that was asked for. An outcome the panel cannot determine is reported as unknown and is never retried automatically, because the first attempt may still be running.

**Done when:** every operation has a durable record of who asked, what they asked for, and how it ended, which survives a restart of the panel. An operator can see in-progress and recent operations for a host. Stopping many guests runs in a stated order with a per-guest result.

**Status:** The operation store, task polling, read-back, recovery and bulk-stop implementation exist. Provider mutation routes currently return `503 agenthub_review_unavailable` before dispatch, so these actions are unavailable in the current build. Earlier disposable-node exercises predate this fail-closed gate and are not current end-to-end evidence. AgentHub authorization must be implemented first; then rerun the lifecycle and failure-recovery matrix. Not built: a view of the hypervisor's own running tasks, and bulk start.

### FO-5 Incus and Proxmox operate as one fleet, and stay separate hosts

**Proxmox and Incus cannot form one cluster.** Their cluster software does not interoperate: there is no shared quorum, no shared state, and no live migration between them. Anything claiming otherwise would fail in the outage it was meant to survive. So a fleet is a grouping in the panel, the same way a vCenter datacenter holds separate clusters, or an OpenStack availability zone groups hosts.

A fleet gives:

- One view of health and capacity across its hosts, each still labelled with its own provider and native cluster.
- Shared tags and placement policy, so new work lands where a rule says, and only on hosts whose provider supports it.
- Failure isolation. A failing or unreachable host affects only that host's row.
- Moving a guest across providers only as an explicit export and import with downtime, never presented as migration.

**Done when:** an operator can group hosts of both providers, sees them separately labelled in one fleet view, and is never offered an action a host's provider cannot do. Native clusters are untouched and keep their own quorum.

**Status:** Partial. Named fleets group hosts of both providers, each labelled with its own provider, version and native cluster, with a rollup of hosts and guests, and actions follow each driver's capabilities. Shared tags, placement policy, capacity totals, operating a whole fleet at once, and cross-provider moves are not built.

### FO-6 Everything is configurable, and owned by the server

What an installation calls things and shows is set in its config file, and owners can override it from the panel with an audited reason. Hiding something does not grant or remove access. Secrets are never in that configuration: it names where they are.

**Done when:** a site can rename and hide navigation, providers and form fields with no code change and no restart, and reset them to the file.

**Status:** Done for navigation, providers and the infrastructure pages. Other pages still have their wording in code; they move to text keys as they are touched.

### FO-7 A host is added from the panel, with no shell on the panel's machine

An operator pastes a Proxmox token or an Incus trust token, tests the connection before saving, and decides whether to trust a self-signed certificate after seeing its fingerprint. A test saves nothing and does not use up a one-time token. Certificate checks are never turned off: trusting a certificate pins exactly that one. Secrets are sealed at rest, never returned, and survive an update.

**Done when:** a host can be added with nothing on the panel's disk, a wrong secret, an unreachable address, a mismatched certificate and a used token each say so, a refused attempt leaves nothing behind, and no secret appears in the database or logs in the clear.

**Status:** Done, and run with one real node of each in the same fleet.

### FO-8 An update keeps what the site has

`git pull --ff-only` and re-running the installer keep the owner, sessions, hosts and saved secrets, and leave the checkout clean for the next pull.

**Done when:** CI installs an older build under systemd, creates the owner, updates, and checks each of those.

**Status:** Done. Rollback is not automated: migrations run forward only, so going back means restoring a backup.

## Order

1. Implement and test the AgentHub action-bound authorization contract; provider mutations remain gated until then.
2. After authorization, finish FO-3, FO-4 and FO-7 against disposable servers: Incus VMs, Proxmox containers, clean shutdown, and real clusters. See the [roadmap](../roadmap.md).
3. FO-5: operating a fleet at once, capacity and placement.
4. FO-2: updating from a button.
