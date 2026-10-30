# Roadmap to a release

Shippin is an early-stage infrastructure platform. The repository contains a static
panel workbench, a draft API contract, governance tooling and an unsigned Debian/Incus
development image builder. It does not yet provide a deployable management system.
The public repository is the source of truth for accepted code; historical material
is a candidate for review, not an implementation dependency.

## Next: prove the primary OS foundation

Complete an unattended test of a **disposable virtual machine** that boots the existing
development ISO, installs Debian and Incus from a verified package mirror, reboots,
and proves an Incus container can start and be removed. Retain command output and
identify failures separately from successes. The test must isolate its disk and use
throwaway credentials. Follow the [implementation handoff](../install/primary-image/implementation-plan.md).
A successful image build or installer boot alone does not establish this milestone.

## Follow-on milestones

1. **Establish service ownership.** Review candidate control-plane and host-agent
   code for provenance, license, security, dependencies and behavior before adopting
   it. Define canonical modules, configuration, contracts and migration boundaries.
2. **Bootstrap a primary.** Build the control plane, persistent data and secure
   first-owner setup. The first owner establishes their own credentials using a
   unique, short-lived bootstrap mechanism; no shared default password. Require
   trusted HTTPS, recovery and a documented backup and restore path. Treat identity
   provider integration as a recoverable multi-step operation.
3. **Enroll infrastructure.** Generate a secondary installer or enrollment command
   from the primary. Authenticate each node using short-lived enrollment credentials
   and unique node identity, protect its connection, allow revocation, and expose
   clear host and VM roles and user-defined tags. Prevent accidental data loss or
   cluster membership changes. Incus is the first provider.
4. **Manage actual workloads.** Implement and test authorized Incus operations,
   isolation and failure handling end to end. Add Proxmox as an explicitly supported
   provider only after its adapter and tests exist. OpenTofu, Coder, chatmaton.io,
   shippin.bot and other services are optional integrations, configured by users.
5. **Package and release.** Produce an installable, versioned image and node
   artifacts through a documented build, with signed artifacts, independently
   distributed verification keys, provenance and an SBOM. Define supported upgrades,
   backup and restore, and a published security response process. Select a license
   before describing the repository as open source or granting reuse rights.

## Release evidence

A release candidate needs evidence from a clean checkout and representative disposable
infrastructure: verified build and artifact signatures; clean primary install and
reboot; first-owner setup and recovery; authenticated enrollment, revocation and
clear tags; real Incus workload lifecycle and isolation; upgrade and rollback or
recovery; backup and restore; and tests for network interruption and invalid or
expired credentials. Document platform, firmware and network prerequisites and known
limitations. Tests must observe system behavior, not only mocked API calls or the
presence of files. No production readiness claim follows from this roadmap alone.
