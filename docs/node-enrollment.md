# Secondary-node enrollment design

This document describes the planned primary-to-node workflow. The current
repository does not implement the issuer, node agent, installer or real node
inventory. The existing [compute API draft](../api/openapi/paths/compute-nodes.yaml)
is the contract starting point; implementation must update it and its tests together.

## Responsibilities

The primary owns operator authentication and authorization, durable enrollment
state, node inventory, audit records and distribution of versioned installer
artifacts. The browser requests enrollment through that control plane; it does
not directly manage a hypervisor. Each secondary runs an agent which connects to
the primary and uses a provider adapter for local infrastructure. Incus is the
first provider; Proxmox is a separate future adapter.

A node represents infrastructure, not a tenant. A request to run tenant work
must be independently authorized and scoped to the tenant and resource. A
heartbeat or configured provider name does not prove isolation, capacity,
workload completion or permission to schedule. Offline, unverified and revoked
nodes must not receive new work.

## Enrollment workflow

1. The primary must be initialized and reachable over trusted HTTPS. An authorized
   operator selects the node name and provider and obtains a short-lived,
   single-node enrollment. The planned API returns the secret once; it must not
   appear in URLs, logs, browser storage or audit records. If lost, revoke the
   enrollment and issue another.
2. Independently verify the signed, versioned installer and the primary's trust
   material before running the installer on the secondary. The installer checks
   OS, architecture, connectivity, time, provider availability and existing host
   configuration. It must not erase disks, reset Incus, alter bridges or firewalls,
   or join a cluster as a side effect of enrollment.
3. The node generates its own private key and submits a certificate signing
   request with the enrollment secret over verified TLS. The primary binds the
   first key to the enrollment. The draft contract permits a bounded retry with
   the same secret and request to recover a lost response; a different key is
   rejected. The node keeps its private key locally.
4. The agent authenticates with its issued identity, reports provider version
   and observed capabilities, and completes non-destructive readiness checks.
   The panel displays state, last contact and reasons; scheduling requires
   verified prerequisites. Node credentials must be rotatable and revocable.

The primary stores a verifier for the one-time secret and records issuance and
revocation without secret values. Rate limits, secure key storage, certificate
rotation, issuer recovery, task authorization and version compatibility require
implementation and independent tests. Revoking an unused enrollment is distinct
from revoking an already enrolled node.

## Existing Incus hosts

Start with independently managed Incus servers. Discover existing instances,
projects, storage and networks without modifying them; select compatible
resources explicitly before scheduling. Independent hosts do not automatically
share storage, networking or failure recovery. An Incus cluster is a separate
operator decision and must not be created or joined by Shippin enrollment. Incus
warns that joining an existing server can lose its existing data; review the
[cluster formation guidance](https://linuxcontainers.org/incus/docs/main/howto/cluster_form/)
for the exact Incus version before any separate cluster operation.

## Proof required

On disposable or explicitly authorized separate machines, demonstrate successful
enrollment and reconnect after restart; denial of expired, replayed, revoked or
wrong-key credentials; recovery from a lost redemption response; rejection of an
untrusted endpoint or artifact; and safe failure while offline. Compare existing
disks, instances, networks and cluster membership before and after installation.
Prove an authorized Incus workload lifecycle and cross-tenant isolation using real
provider state. Record skipped checks and failures rather than interpreting a
mock response or agent heartbeat as a successful deployment.
