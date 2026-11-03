# Fleet operations

What an operator running Shippin against real Incus and Proxmox servers needs from the panel, stated as requirements a change can be checked against. The status of each is what the code does today.

## Terms

- **Host (node):** one hypervisor API the panel can reach. It is always exactly one provider.
- **Provider:** a kind of hypervisor, such as Incus or Proxmox VE. A driver in `internal/hv` implements it.
- **Guest:** a virtual machine or container on a host.
- **Native cluster:** a cluster the hypervisor forms itself. Proxmox uses corosync and pmxcfs. Incus uses its own raft database. They own quorum, shared state and live migration.
- **Fleet:** a group of hosts in the panel, for operating them together. It is a management-plane idea and changes nothing on the hosts.

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

**Status:** Done for provider, version, reachability and guest counts. Kernel, uptime, CPU and memory per node, and the members of a native cluster, are not built.

### FO-4 Lifecycle operations are reliable and visible

Starting or stopping a guest is not finished when the request is accepted. It is finished when the hypervisor's task has ended and a fresh read shows the state that was asked for. An outcome the panel cannot determine is reported as unknown and is never retried automatically, because the first attempt may still be running.

**Done when:** every operation has a durable record of who asked, what they asked for, and how it ended, which survives a restart of the panel. An operator can see in-progress and recent operations for a host. Stopping many guests runs in a stated order with a per-guest result.

**Status:** Partial. Both drivers wait for the task and read back, and every change is audited before it runs. The durable operation history, its UI and ordered bulk stop are not built.

### FO-5 Incus and Proxmox operate as one fleet, and stay separate hosts

**Proxmox and Incus cannot form one cluster.** Their cluster software does not interoperate: there is no shared quorum, no shared state, and no live migration between them. Anything claiming otherwise would fail in the outage it was meant to survive. So a fleet is a grouping in the panel, the same way a vCenter datacenter holds separate clusters, or an OpenStack availability zone groups hosts.

A fleet gives:

- One view of health and capacity across its hosts, each still labelled with its own provider and native cluster.
- Shared tags and placement policy, so new work lands where a rule says, and only on hosts whose provider supports it.
- Failure isolation. A failing or unreachable host affects only that host's row.
- Moving a guest across providers only as an explicit export and import with downtime, never presented as migration.

**Done when:** an operator can group hosts of both providers, sees them separately labelled in one fleet view, and is never offered an action a host's provider cannot do. Native clusters are untouched and keep their own quorum.

**Status:** Partial. Both providers appear side by side, separately labelled, on the overview, and actions already follow each driver's capabilities. Named fleets, placement policy and cross-provider moves are not built.

### FO-6 Everything is configurable, and owned by the server

What an installation calls things and shows is set in its config file, and owners can override it from the panel with an audited reason. Hiding something does not grant or remove access. Secrets are never in that configuration: it names where they are.

**Done when:** a site can rename and hide navigation, providers and form fields with no code change and no restart, and reset them to the file.

**Status:** Done for navigation, providers and the infrastructure pages. Other pages still have their wording in code; they move to text keys as they are touched.

## Order

1. FO-4: the operation history. Everything else about trust depends on it.
2. FO-3: node detail.
3. FO-5: named fleets.
4. FO-2: updating from a button.
