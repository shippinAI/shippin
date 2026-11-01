# VM-first tenants

Status: design + first implementation, 2026-10-16. Decision owner: the operator.
Guesses and unverified points are marked **[guess]**; verified on Gold are marked **[verified]**.

## 1. Decision

Every *new* tenant gets one full Incus VM (KVM) — the **tenant VM** `tvm-<uuid>` — inside its existing
silo (project `t-<uuid>`, OVN network `n-<uuid>`, encrypted dataset `tank/tenants/<uuid>`, pool `p-<uuid>`).
Every new workspace of that tenant is an unprivileged container **nested inside the tenant VM**, run by
an Incus daemon inside the VM. The VM, not the host kernel, becomes the tenant's isolation boundary.

Existing container workspaces keep running exactly as today ("**legacy direct**": container directly in
`t-<uuid>` on the host). Nothing migrates without the operator's approval (section 11).

Why: containers share the host kernel; a kernel bug or container escape on Gold reaches every tenant
and the platform (console, Nomen, Postgres). With a VM per tenant an escape from a workspace lands in
that tenant's own guest kernel. This is the standard pattern for multi-tenant dev platforms that let
users run arbitrary code (Firecracker/microVM per tenant at Fly, Gitpod, Codespaces VMs; LXD/Incus docs
recommend VMs for untrusted workloads). The cost is RAM/CPU overhead per tenant and slower first
provisioning — acceptable for paid tenants, and the plan config controls sizes.

## 2. KVM on Gold [verified 2026-10-16]

`/dev/kvm` present (root:kvm 0660), `kvm_intel` loaded, 192 vmx flags (96 threads), nested=Y,
376 GiB RAM (≈345 free), tank 5.44 T (5.42 T free). A smoke VM had already run in the Incus build.

## 3. Tenant VM spec (all from plan config, nothing hardcoded)

New plan keys in `/etc/shippin/hostd.json` → `plans.<plan>.vm`:

| key | meaning | founder |
|---|---|---|
| `cpu` | vCPUs (`limits.cpu`) | 16 |
| `memory` | `limits.memory` | 32GiB |
| `root_gib` | VM root disk | 20 |
| `data_gib` | data disk = inner Incus pool | 150 |
| (inner defaults) | inner `ws` profile limits | plan `workspace_cpu`/`workspace_memory` |

The project quota (`limits.disk.pool.p-<uuid>`) still caps the sum. Project `limits.virtual-machines`
becomes 1 (was 0). `limits.cpu`/`limits.memory` of the project already equal the VM size, so the VM uses
the whole tenant budget; inner limits subdivide it. `security.secureboot` stays on (Debian cloud images
are signed). `migration.stateful` off.

## 4. Image

`shippin/tenant-vm`, built by `deploy/image/build-tenant-vm.sh` from the official `images:debian/13/cloud`
VM image (copied once to the local image store as `shippin/debian13-cloud-vm`):
- `incus-base` from Zabbly lts-6.0 (same 6.0.x line as Gold; key fingerprint pinned and checked in the
  build). `incus-base` = daemon without the UI/extra clients.
- `btrfs-progs`, `net.ipv4.ip_forward=1`, `/usr/local/sbin/shippin-tvm-init`.
- **No** `core.https_address`: the inner API never listens on a network (section 7).
- cloud-init cleaned so each VM gets its own machine-id and SSH host keys. No user accounts, no SSH
  daemon needed for control.

Templates (`shippin/tpl-*`) are not baked in: hostd exports each template from the host image store once
(`/var/lib/shippin/images/<fingerprint>.tar.gz`, cached), pushes it into the VM and imports it under the
same alias. Updating a template = new fingerprint → re-imported lazily on next workspace create.

## 5. Storage

- VM root disk: zvol on `p-<uuid>` → inside the tenant's encrypted dataset. Encryption at rest is
  unchanged (key in sops, loaded by `shippin-zfs-unlock`).
- Inner storage: a second block volume `tvd-<uuid>` (custom volume, `--type=block`, `data_gib`) on
  `p-<uuid>`, attached as device `data`; it shows up in the guest as
  `/dev/disk/by-id/scsi-0QEMU_QEMU_HARDDISK_incus_data`. The inner Incus pool `default` is **btrfs** on
  it. btrfs over ZFS-in-guest because ZFS in the guest needs DKMS and duplicates ARC caching; btrfs
  gives inner snapshots, quotas (`size=` on inner root/home) and online grow. **[guess]** btrfs quota
  accounting overhead is acceptable at our scale (qgroups are known to slow down with thousands of
  snapshots; we don't keep many inner snapshots).
- Separate data disk (instead of the root disk) so the inner pool can grow without touching the guest
  OS and so a rebuild of the VM image can re-attach the same data disk.
- Home volumes `h-<uuid>` live in the inner pool, attached at `/home` exactly as today.

## 6. Networking

```
edge (hub 198.51.100.10) ──peer── n-<t> (OVN, 10.128.X.0/24)
                                  └─ tvm-<t> eth0 10.128.X.2   (static ipv4.address)
                                       ipv4.routes = 10.128.Y.0/24  ← OVN routes the inner /24 to the VM
                                       └─ incusbr0 10.128.Y.1/24 (inside VM, nat for egress only)
                                            └─ w-<uuid> 10.128.Y.N (unprivileged container)
```
Choice: **routed inner subnet**, not a VM-side proxy.
- Each tenant VM gets a second /24 from the same `tenant_cidr` (10.128.0.0/9), recorded on the
  network as `user.shippin.inner_subnet` so the allocator never hands it out twice.
- OVN NIC `ipv4.routes` makes the tenant router send that /24 to the VM; the VM forwards to its bridge.
- The edge already routes 10.128.0.0/9 into the hub peering, so IDE (`:3000`), previews
  (`<port>--<slug>.ws.shippin.cloud`), SSH DNAT (192.0.2.252:22xxx → inner IP:22) and published ports
  work with the inner container IP exactly as with a legacy container IP. Caddy routes, forward-auth,
  cookie strip and the consent checks in the console are untouched: they key on workspace id/host,
  not on placement.
- The tenant ACL `ws-ingress` is enforced on the VM's OVN port, so only the edge (HubIP) can reach
  3000/22/preview ports, and per-workspace `ws-*` rules keep their destination = inner IP.
- Inner bridge keeps `ipv4.nat=true` only for egress (new outbound flows are masqueraded to the VM
  address and then SNATed by OVN); inbound from the edge is routed, not NATed, so the workspace sees the
  real edge source and the ACL sees the real destination.
- Rejected: a VM-side reverse proxy/agent. It would need per-workspace port maps (every workspace wants
  :3000 and :5173), a new daemon with its own auth, and SSH would need a jump. Routing is simpler and
  keeps the edge as the single policy point.
- OVN peering forwards traffic for the NIC `ipv4.routes` subnet **[verified 2026-10-16]**: edge →
  inner container ping and `:3000` 200 with no extra hub routes; Gold itself cannot reach the inner IP.
- MTU: the tenant OVN network is 1442. systemd-networkd in the Debian cloud image ignores the DHCP MTU
  by default, so the guest came up at 1500 and large packets stalled **[verified]**. The image sets
  `UseMTU=yes` and `shippin-tvm-init` pins `MTUBytes` to the network's `bridge.mtu`; the inner bridge
  uses the same MTU.

## 7. Control: how hostd drives the inner Incus

Choice: **`incus exec tvm-<t> -- incus …` through the Incus VM agent (virtio-vsock)**.
- No inner API listener exists, so a workspace container (or anything on the tenant network) cannot
  even attempt to reach the inner Incus API. With the API-over-network option the inner daemon would
  listen on 10.128.X.2:8443 — reachable from the tenant's own containers through the VM bridge unless
  firewalled perfectly — and we would have to mint, store (sops), rotate and revoke a client cert per
  tenant, plus trust the VM's server cert. More moving parts, more secrets, bigger attack surface.
- The vsock channel is host → guest; hostd authenticates the guest implicitly (Incus picks the VM by
  name), and the guest gains nothing on the host. hostd keeps its rules: argv only, no shell, a closed
  allowlist of commands. The wrapper (`internal/hostd/inner.go`) takes the same argv hostd uses on the
  host and prefixes `exec tvm-<t> --project=t-<t> --`.
- **Output from the guest is untrusted** (a tenant who escaped into its VM controls it): hostd only
  parses it as JSON into typed structs, size-bounded, and IPs returned are checked to be inside the
  tenant's inner /24 before they are published to the edge (so a compromised guest cannot point a route
  at another tenant).
- Trade-off: `incus exec` needs the agent running; if a tenant breaks its VM the control plane loses
  it, which is the correct failure mode (only that tenant is affected; operator can restart the VM).

## 8. Lifecycle, resizing, growth

- Provision: dataset → pool → project (VMs=1) → network → hub → profile → **VM**: data volume, init from
  `shippin/tenant-vm`, static eth0 IP + `ipv4.routes`, start, wait for agent, `shippin-tvm-init`.
  Each VM step is idempotent; a failure in the VM phase rolls back what this attempt created (VM, data
  volume) and the tenant shows the error with Retry (existing `/v1/tenants/{id}/provision`).
- Grow disk: `POST /tenants/{t}/storage` or `shippin-hostd storage add` raises the dataset quota,
  the project pool limit, the VM root, the data volume, and the inner btrfs (`shippin-tvm-init grow`).
  `POST /tenants/{t}/plan` does the same up to a tier. Shrinking is refused. CPU and memory are
  written onto the VM and not restarted (`restart_required` on the response). Low space is on
  `GET /tenants/{t}/storage` and `GET /tenants/{t}/vm`. Details: `docs/architecture/org-storage.md`.
- Stop/start of the VM stops/starts workspaces that were running. On Incus 6.0 (what gold runs)
  `boot.autostart` is left unset, which restores last power state; a stopped workspace stays
  stopped. Incus 6.21 added an explicit `last-state` value that 6.0 rejects, and `true` means
  always start. Do not set `true`. Activation containers (`u-<account>`) are started only by the API.

## 9. Backups and snapshots

- Crash-consistent: ZFS snapshot of `tank/tenants/<uuid>` (recursive) covers VM root + data volume;
  `incus snapshot create tvm-<t>` does the same per VM. Sent encrypted (`zfs send -w`) offsite so the
  backup host never sees plaintext.
- App-consistent per workspace: inner `incus snapshot` / `incus export` of a workspace (btrfs).
- Planned schedule (not implemented yet): daily `snapshots.schedule` on the VM, 7 kept.

## 10. Security

- Isolation boundary = KVM. Inner containers are unprivileged with `security.idmap.isolated=true` in the
  inner project `ws` (`restricted=true`, nesting/lowlevel/interception/proxy/unix/gpu/usb/pci blocked,
  `limits.virtual-machines=0`) — the same restrictions as the host tenant projects today.
- The VM itself: no `security.nesting` (meaningless for VMs), `restricted.virtual-machines.lowlevel=block`
  stays on the host project, no raw qemu config, no passthrough devices.
- Consent/break-glass, forward-auth and audit are in the console and edge and do not change.
- Remaining host attack surface per tenant: QEMU + virtio devices + vsock agent. Keep Incus/QEMU patched
  (unattended-upgrades on Gold).

## 11. Migration of existing tenants

- A tenant is VM-backed iff `user.shippin.mode=vm` is set on its project (set when the VM is created).
- Workspace placement is per workspace: an instance found in the host project is **legacy direct**;
  otherwise it lives in the VM. Both kinds work side by side in one tenant.
- New tenants: VM by default (`vm_first: true` in hostd config). Existing tenants (the operator 01a14149-…,
  WiredFRONT, Access Test Co) get **no** VM and keep creating legacy containers until an operator runs
  "Enable tenant VM" — nothing happens automatically, `dev` is untouched.
- Moving one legacy workspace into the VM (later, with the operator's approval): stop → `incus export` →
  push into VM → `incus import` → new inner IP → `sync` republishes route/previews/ports. SSH host keys
  and home volume travel with the export. Downtime ≈ export size / disk speed. Not implemented yet.

## 12. Open decisions for the operator
1. Default VM size: VM takes the whole plan budget (16 vCPU/32 GiB for founder). RAM is reserved by
   QEMU as the guest touches it; with ballooning off it is not returned. Fine for a few tenants, not for
   hundreds of free tiers.
2. Whether to enable a VM for his own tenant and migrate `dev`.
3. Backup schedule/retention and offsite target.

## 13. Verification log (2026-10-16, throwaway tenants only)
- hostd-level tenant `019a0000-…f001` and console tenant "VM Test Co" (org vm-test): VM provisioned in
  ~75 s, inner Incus initialised, `shippin/tpl-node` imported into the VM on first use (~30 s).
- Node/Vite workspace from github.com/RicardoValdovinos/vite-react-boilerplate inside the VM: IDE 302
  unauthenticated, 200 for the owner via edge; preview `5173--…` 302 / 200 with the Vite page; SSH via
  192.0.2.252:22002 lands in the nested container; egress OK; inner and VM Incus API unreachable.
- `deploy/tests/access-live.sh` against the nested workspace: 37/37 (consent still blocks the admin).
- VM stop → workspace row stopped; VM start → autostarted, row running, IDE/preview 200 again;
  workspace stop/start inside the VM OK.
- Tenant delete removed workspace, edge route/ports, VM, data volume, project and pool (dataset kept).
- Provisioning rollback seen live: a failed VM init rolled back the VM and the new network; the retry
  succeeded.
See docs/reviews/2026-10-16-vm-first.md and the build log.
