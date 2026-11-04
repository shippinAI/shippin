# Roadmap

What works today, what does not, and what comes next. [The objective](objective.md) says where this is going.

## Works today

- Deploy with `docker compose up`, or on Debian or Ubuntu with the installer. CI proves both from a clean checkout.
- First owner and sign-in, with or without an upstream identity provider. Orgs, invites and a waitlist.
- Infrastructure: an overview and one tab per provider, for Incus and Proxmox. Add a host from the panel with a Proxmox token or an Incus trust token, test it first, and trust a self-signed certificate by its fingerprint. Start, stop and snapshot guests, stop many with a result for each, and see every change in a history that survives a restart. Node detail shows version, kernel, CPU, memory and the host's own cluster. Fleets group hosts of any provider.
- Updates: `git pull --ff-only` and re-running the installer keep the owner, sessions, hosts and saved secrets. CI proves it from an older build.
- The control plane: a separate system tenant, a permission for each infrastructure duty, a required note on every change, and an audit log the panel's database role cannot rewrite on an installer deployment. See [the control plane](design/control-plane.md).
- Workspaces and projects, an API explorer, an agent hub, a provider panel, and audited platform settings.
- Names, navigation and providers are configuration, and owners can change them from the panel.
- The panel shows its running build and tells operators when a newer one has been pushed.

## Known gaps

- **Hypervisors:** adding a host, node detail, start, stop, shutdown, reboot, snapshot, restore and bulk stop have been run against Incus 6.0.4 and Proxmox VE 9.2, each on one node in a throwaway container. Incus guests were containers and Proxmox guests were QEMU VMs with no operating system. Not yet run: Incus VMs, Proxmox containers, a guest with a real operating system answering a clean shutdown on Proxmox, and any real cluster. Try it on a disposable guest first.
- **Passkeys and certificates:** browsers refuse passkeys on a site with a certificate warning. With a self-signed certificate, no mail and no identity provider, each sign-in uses up one of eight recovery codes, and they do not renew. Use a real certificate, a provider or mail for anything you intend to keep.
- **Rollback:** migrations run forward only. Going back to an older build means restoring a backup.
- **Local agent:** neither deployment path sets up `shippin-hostd`. The features that depend on it, including workspaces on the primary, need `incus.hostd_url` set and hostd installed by hand. See `deploy/hostd-install.sh`.
- **Remote nodes:** node enrollment exists for a token and a heartbeat. There is no node CA and no installer for a secondary. See [node enrollment](node-enrollment.md).
- **Change approvals:** the Gold signing code is still in the tree, off unless configured. It must be retired through a reviewed migration. See [the plan](system-identity-human-review-plan.md).
- **Reference panel:** `third_party/warden-ct102` still has features to port. See [the ledger](panel-variants.md).
- **Wording:** pages other than infrastructure and navigation still have their text in code.
- **Primary image:** the ISO builder in `install/primary-image/` is unsigned and does not install the panel.

## Next

The order is in [fleet operations](design/fleet-operations.md):

1. Run what is listed under Hypervisors above, and make the run against real servers repeatable in CI.
2. Finish the [control plane](design/control-plane.md): separating who asks from who approves, and backing up the panel from the panel.
3. Operating a fleet at once, capacity totals and placement.
4. Updating from a button.

Then secondary-node installation, signed releases, backup and restore with a tested restore, and managed data services. Those are [designed](design/managed-services.md) but not built.

## Release gates

A release needs evidence from a clean checkout and disposable infrastructure: install and reboot, first-owner setup and recovery, real Incus and Proxmox workload lifecycle, upgrade and rollback, backup and a tested restore, and behavior under network loss and expired credentials. Tests must observe system behavior, not only mocked API calls.
