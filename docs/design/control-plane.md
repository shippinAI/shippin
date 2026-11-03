# The control plane

How the panel separates the platform from its customers, who may change what, and how a change is accounted for. Each requirement can be checked against a change, and says whether it is built.

## The model

A reserved **system tenant** owns what runs the platform: the hosts added from the panel, the guest the panel runs on, and later the platform's own workspaces and databases. This is the pattern behind Cloud Foundry's `system` org, Rancher's `local` cluster and `System` project, and an AWS organization's management account.

Three rules keep it safe:

1. **Owning is not access.** Belonging to the system tenant grants nothing. Power comes from staff roles on a separate plane, so a mistake in tenant membership cannot hand anyone the platform.
2. **Customer paths cannot reach it.** The customer reads and lifecycle actions in the database layer leave it out, so a route that forgets to check still cannot list, suspend or delete it.
3. **The control plane does not depend on it.** Signing in, authorizing and auditing work whatever state that row is in. A tenant problem must never lock the operator out of fixing it.

```
Staff roles ─────────grant──▶ permissions ──gate──▶ every /v1/infra and /v1/operator route
                                                          │ note + audit, then approval
                                                          ▼
System tenant ──owns──▶ hosts, the panel's guest      Customer tenants ──own──▶ their own servers
      (kind = system, never listed to customers)            (see only what is theirs or assigned)
```

## Requirements

### CP-1: The platform is a tenant, marked apart

**Status:** Done. `tenants.kind` is `customer` or `system`, and a unique index allows one system tenant. It cannot be suspended, deleted, granted access to or joined through an org id. It appears on the Tenants page as its own card, not in the customer list or counts.

### CP-2: A customer cannot see the platform's hosts

**Status:** Done. A tenant sees its own servers and any host assigned to it for placement, which it can read but not change. Before this, every customer could list every platform host.

### CP-3: Each duty has its own permission

**Status:** Done for infrastructure; the provider panel already worked this way. Every `/v1/infra` route names one catalog permission, and a route without one stops the panel from starting.

| Permission | Allows |
| --- | --- |
| `infra.read` | Hosts, guests, node detail, Activity |
| `infra.hosts.write` | Add, test and detach hosts |
| `infra.fleets.write` | Create and change fleets |
| `infra.guests.power` | Start, stop, shut down, reboot, bulk stop |
| `infra.guests.snapshot` | Take snapshots |
| `infra.guests.restore` | Restore from a snapshot or backup |
| `infra.guests.backup` | Back up guests |
| `infra.guests.manage` | Create, clone, reconfigure, resize, migrate, delete |
| `infra.storage.write` | Download and upload to hypervisor storage |
| `infra.self` | Change the guest this panel runs on |

Staff roles grant these once the team is in enforce mode (**Provider panel → Team**), and staff permissions always need a second factor. The founder role, the local installation owner and `roles.infra` hold all of them, so a single-owner install keeps working with no identity provider. Presets: **Owner** has everything, **Infrastructure** has everything except `infra.self`, **SRE / Ops** can read, power and snapshot, and **Security** and **Auditor** can read.

### CP-4: Every change says why

**Status:** Done for infrastructure. A change to a host or guest needs a note of 5 to 300 characters, sent as the `X-Change-Note` header, percent-encoded. Without one the server answers `400` with code `note_required` before anything happens. The note goes in the audit log, on the approval card and in Activity. Fleets change nothing on a host, so they are audited without a note.

**Not built:** notes on the provider panel's own risky actions, such as suspending a tenant. Those are covered by staff mode's reason for the session, not one per change.

### CP-5: The audit log can't be rewritten by the panel

**Status:** Done on the installer path. Each run hands `audit_log` to a role nobody can log in as, so the panel's database role can only read and append, and adds a hash chain that can be re-walked. The system tenant's card shows whether it is protected and intact. CI runs the tests that prove the role cannot change or delete entries.

**Not built:**
- Protection under `docker compose`, where the panel's database user is a superuser. The card says so.
- Shipping the chain's head somewhere the database's own superuser cannot reach. Until then someone with root on the database can still rewrite history and the chain together.

### CP-6: The panel can run on the server it manages

**Status:** Partly done. The panel attaches to its own hypervisor like any other host, the host is marked **Runs this panel**, and its own guest cannot be stopped, restored, deleted or migrated without a confirmation and `infra.self`. Bulk stop always skips it.

**Not built:** patching and rebooting the host itself, backing up and restoring the panel from the panel, and a documented way back in when the panel is down. Recovery today is the hypervisor's own tools. This should stay true: a control plane must never be the only way to repair itself.

### CP-7: Asking and approving are different people

**Status:** Not built. An approval covers one change, but the person who asked can approve it. That is the only option for a sole owner. With a team, it should take a second person, with self-approval only as an explicit setting.

### CP-8: The platform acts under its own identity

**Status:** Not built. The panel should hold a machine identity in the identity provider, separate from any person and with no interactive sign-in, and should fetch hypervisor secrets from the secrets service at use rather than keeping them sealed in its own database. Humans must not sign in as it.

### CP-9: Each surface has its own domain

**Status:** A deployment choice; the panel already takes all of this from configuration.

- The panel gets its own registrable domain. Session cookies stay host-only.
- Customer workloads never share a registrable domain with the panel or with marketing. Put them under a dedicated name registered on the [Public Suffix List](https://publicsuffix.org/), so one customer's site cannot set cookies for another's. This is why GitHub serves pages from `github.io`.
- Mail uses its own domain or subdomain, so that a deliverability or reputation problem cannot affect the panel.

## Order

1. CP-7, so a team can actually separate duties.
2. CP-4 on the provider panel's risky actions.
3. CP-5 off-box anchoring and containers.
4. CP-6 backup and restore of the panel.
5. CP-8, once the identity and secrets services are attached.
