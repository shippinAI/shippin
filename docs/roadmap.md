# Roadmap

What works today, what does not, and what comes next. [The objective](objective.md) says where this is going.

## Works today

- Deploy with `docker compose up`, or on Debian or Ubuntu with the installer. CI proves both from a clean checkout.
- First owner and sign-in, with or without an upstream identity provider. Orgs, invites and a waitlist.
- Infrastructure: an overview and one tab per provider, for Incus and Proxmox. Add and detach hosts, and start, stop and snapshot guests.
- Workspaces and projects, an API explorer, an agent hub, a provider panel, and audited platform settings.
- Names, navigation and providers are configuration, and owners can change them from the panel.
- The panel shows its running build and tells operators when a newer one has been pushed.

## Known gaps

- **Hypervisors:** the Incus and Proxmox drivers have only been exercised against simulated APIs in tests. Try them on a disposable guest first.
- **Local agent:** neither deployment path sets up `shippin-hostd`. The features that depend on it, including workspaces on the primary, need `incus.hostd_url` set and hostd installed by hand. See `deploy/hostd-install.sh`.
- **Remote nodes:** node enrollment exists for a token and a heartbeat. There is no node CA and no installer for a secondary. See [node enrollment](node-enrollment.md).
- **Change approvals:** the Gold signing code is still in the tree, off unless configured. It must be retired through a reviewed migration. See [the plan](system-identity-human-review-plan.md).
- **Reference panel:** `third_party/warden-ct102` still has features to port. See [the ledger](panel-variants.md).
- **Wording:** pages other than infrastructure and navigation still have their text in code.
- **Primary image:** the ISO builder in `install/primary-image/` is unsigned and does not install the panel.

## Next

The order is in [fleet operations](design/fleet-operations.md):

1. A durable history of lifecycle operations, and a view of what is running.
2. Node detail: kernel, uptime, capacity, and native cluster members.
3. Named fleets that group Incus and Proxmox hosts.
4. Updating from a button.

Then secondary-node installation, signed releases, backup and restore with a tested restore, and managed data services. Those are [designed](design/managed-services.md) but not built.

## Release gates

A release needs evidence from a clean checkout and disposable infrastructure: install and reboot, first-owner setup and recovery, real Incus and Proxmox workload lifecycle, upgrade and rollback, backup and a tested restore, and behavior under network loss and expired credentials. Tests must observe system behavior, not only mocked API calls.
