# Shippin release objective

Build a self-hosted infrastructure platform that an operator can install on a
primary server, open in a browser, set up with their own account, and use to
securely connect and manage additional servers or VMs. Shippin is one panel
for customer and operator workflows; it must support both Incus and Proxmox servers.
The public Shippin repository is the canonical
owner of accepted platform code.

## First usable release

A release candidate must demonstrate the following together on disposable or
otherwise explicitly authorized infrastructure:

1. **Install and recover the primary.** Install a versioned Shippin primary on
   Debian, establish the first owner over trusted HTTPS with a unique setup
   mechanism, restart it, and sign in again. Provide a tested backup and restore
   path. No shared factory password.
2. **Connect an existing secondary.** From that primary, issue a short-lived
   enrollment command or installer and securely enroll a separately installed
   Debian server or VM. Identify its role and configurable tags. Enrollment must
   not silently erase data or join an Incus cluster. Test expiry, replay,
   revocation and disconnected-node behavior. Existing-host adoption and its
   supported configurations must be implemented and verified; this is a goal,
   not a present guarantee.
3. **Operate real provider workloads.** Display truthful node readiness and use
   the panel and control plane to create, inspect, stop and remove an isolated
   workload on enrolled Incus and Proxmox nodes, according to each provider's
   supported capabilities. Prove the actual server state and authorization
   boundaries, including failure and recovery; mock API responses alone do not
   satisfy this gate.
4. **Deliver and maintain it.** Document installation prerequisites, pull or
   release deployment, updates and recovery. Produce versioned, authenticated
   artifacts with independently verifiable provenance and a software bill of
   materials. Choose a license and publish an explicit support and security policy
   before offering a release.

## Boundaries and sequencing

Keep one source owner per component and use the existing API contract as the
starting point, updating it alongside implementation. Review historical code for
provenance, security, dependencies and tests before adopting it; preserve its
private context. Do not hardcode a particular customer's addresses, accounts or
services. UI settings must reflect actual authorized server-side configuration.

Build the primary control plane, identity and durable state alongside the
installer proof, then secure enrollment and provider lifecycle in verifiable
increments. Both Incus and Proxmox are release goals; implementing one first does
not establish support for the other. OpenTofu/Terraform, Coder, AgentHub, Chatmaton and shippin.bot are
optional integrations, not prerequisites for the first usable release. Support
for any integration requires its own implementation and tests.

The [roadmap](roadmap.md) says what works today and what comes next, and the
[source boundaries](source-of-truth.md)
for ownership. The [service context map](service-contexts.md) defines the intended
managed data, identity, storage, communication and agent boundaries.
