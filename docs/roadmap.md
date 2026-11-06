# Roadmap

What works today, what does not, and what comes next. [The objective](objective.md) says where this is going.

## Works today

- Deploy with `docker compose up`, or on Debian or Ubuntu with the installer. CI proves both from a clean checkout.
- First owner and sign-in, with or without an upstream identity provider. Orgs, invites and a waitlist.
- Infrastructure: an overview and one tab per provider, for Incus and Proxmox. Add a host from the panel with a Proxmox token or an Incus trust token, test it first, and trust a self-signed certificate by its fingerprint. Inventory, node detail, capacity and fleets are available. Single-guest start, stop, shutdown and reboot now require a persisted exact-action AgentHub review and one-shot execution record in the existing panel; batch and other provider mutations still fail closed. Live AgentHub policy/quorum and hypervisor execution are not verified. See [configurable human review](system-identity-human-review-plan.md).
- Updates: `git pull --ff-only` and re-running the installer keep the owner, sessions, hosts and saved secrets. CI proves it from an older build.
- The control plane: a separate system tenant, configured infrastructure permissions, and an audit log the panel's database role cannot rewrite on an installer deployment. Guest power actions have a persisted Shippin review/event record, but deployment-backed immutability and live least-privilege AgentHub/provider behavior still need verification. See [the control plane](design/control-plane.md).
- Workspaces and projects, an API explorer, an agent hub, a provider panel, and audited platform settings.
- Names, navigation and providers are configuration, and owners can change them from the panel.
- The panel shows its running build and tells operators when a newer one has been pushed.

## Known gaps

- **Hypervisors:** host enrollment and read-only inventory remain available. Although provider drivers, operation records and lifecycle routes exist, every provider mutation currently returns `503 agenthub_review_unavailable` before a driver call. Earlier disposable-node lifecycle exercises predate this gate and do not prove that mutations work in the current build. AgentHub's typed action/system/resource authorization and service identity must be integrated before lifecycle execution can be re-enabled. Then repeat tests for Incus containers and VMs, Proxmox QEMU and LXC, clean shutdown, snapshots, task read-back, denial, timeout, restart and real clusters on disposable infrastructure.
- **Passkeys and certificates:** browsers refuse passkeys on a site with a certificate warning. With a self-signed certificate, no mail and no identity provider, each sign-in uses up one of eight recovery codes, and they do not renew. Use a real certificate, a provider or mail for anything you intend to keep.
- **Rollback:** migrations run forward only. Going back to an older build means restoring a backup.
- **Local agent:** neither deployment path sets up `shippin-hostd`. The features that depend on it, including workspaces on the primary, need `incus.hostd_url` set and hostd installed by hand. See `deploy/hostd-install.sh`.
- **Remote nodes:** node enrollment exists for a token and a heartbeat. There is no node CA and no installer for a secondary. See [node enrollment](node-enrollment.md).
- **Reference panel:** `third_party/warden-ct102` still has features to port. See [the ledger](panel-variants.md).
- **Wording:** pages other than infrastructure and navigation still have their text in code.
- **Primary image:** the ISO builder in `install/primary-image/` is unsigned and does not install the panel.

## Next

The order is in [fleet operations](design/fleet-operations.md):

1. Implement and verify the AgentHub review contract and least-privilege Shippin identity. Provider mutations remain disabled until that is proven.
2. Re-enable lifecycle through the review gate, then run the Hypervisors matrix above and make disposable-provider workflows repeatable in CI.
3. Implement bounded deployment reconciliation and source builds, then managed data services and their backup/restore flows.
4. Finish fleet capacity totals and placement, then updating from a button with health checks and rollback.

For source-folder closure sequencing (console/cloud/mesh/Warden imports), use the
[source-closure worklist](platform-consolidation-plan.md#source-closure-worklist).

Then secondary-node installation, signed releases, backup and restore with a tested restore, and managed data services. Those are [designed](design/managed-services.md) but not built.

## Release gates

A release needs evidence from a clean checkout and disposable infrastructure: install and reboot, first-owner setup and recovery, real Incus and Proxmox workload lifecycle, upgrade and rollback, backup and a tested restore, and behavior under network loss and expired credentials. Tests must observe system behavior, not only mocked API calls.
