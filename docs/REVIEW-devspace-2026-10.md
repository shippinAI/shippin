# Devspace review, 2026-10-22

Review of the devspace, tenant-VM, workspace, and AI-terminal paths on `b2-release`
(`9059c93`). Live hostd on gold is the parent commit `90aa806`. Nothing here was deployed.

## Which branch a PR should target

| Branch | Tip | Relation to gold |
|---|---|---|
| `main` | `93dd341` | Ancestor of `b2-release`. Missing connect, publish, usage, R-J1a, B2, and B3. Not what gold runs. |
| `b2-release` | `9059c93` | Hostd release line. Live hostd `90aa806` plus one commit (site publish gate, default off). |
| `approve-p1-only` | `fa18185` | Fast-forward of `b2-release` plus gold-sign / click-approve. Live console *base*. No unique hostd commits. |
| (none) | `f64b96c` | Live console *build* (founder-role). Not in this repository. |

PRs for hostd and this foundation target **`b2-release`**. That is the branch gold's hostd is built from.
`approve-p1-only` should merge `b2-release` forward when the console is next cut; targeting it directly
would put a devspace change on the approvals line. `main` is behind production.

## What is in good shape

The vm-first control plane is coherent. A new tenant with a plan that has a `vm` block gets an Incus
project, an encrypted ZFS dataset, one KVM VM, and workspaces as unprivileged containers inside that
VM. `place` sends Incus calls through `incus exec tvm-<t> -- incus …` on the VM agent (vsock). The
inner daemon has no network listener (`deploy/image/tenant-vm/shippin-tvm-init` does not set
`core.https_address`). Guest stdout is capped at 8 MiB and decoded into typed fields. An address is
published to the edge only if it sits in that VM's inner /24 (`place.checkIP`).

Host projects are `restricted=true` with nesting, low-level, interception, proxy, and device
passthrough blocked, and `limits.virtual-machines=1`. Inner projects (`ws` and `pj-<uuid>`) mirror
that and set `limits.virtual-machines=0` plus `security.idmap.isolated=true`. A failed VM create rolls
back the VM and data volume from that attempt.

The edge is the policy point for browser traffic. Caddy `forward_auth`s the IDE and `/_term/` to the
console `/auth/ws-check` and strips the workspace session cookie before the upstream. The IDE binds
the container's eth0 address, not `0.0.0.0` (`deploy/image/shippin-ide`). The tenant ACL drops
ingress except from the hub.

Credentials: hostd never puts a token on an argv. `shippin-credwriter` writes mode `0600` files under
`~/.ai/connect`, discards vendor CLI output, and the image build refuses to publish if login-like
files were baked in. The AI home is a volume on the tenant's encrypted ZFS dataset (the inner btrfs
pool is a block volume on that dataset). Encryption is ZFS, not an application cipher. Agent argv is
an allowlist; `--yolo`, `--always-approve`, `--dangerously-skip-permissions`, and
`--dangerously-bypass-approvals-and-sandbox` are rejected.

Gitleaks is pinned (`8.28.0` plus sha256). The publish token does not enter the devspace.

## Findings

Severity is about a tenant on gold today, not about the target design.

### Critical

**C1. Inside one tenant VM, workspace ports have no auth.**

openvscode-server runs `--without-connection-token`. ttyd (activation, port 7681) and the agent
terminal (port 7682) do too. The edge and the OVN ACL stop other tenants and the public internet.
They do not stop another container on the same inner bridge `incusbr0`. Any process in a sibling
workspace, or in the activation container, can open `:3000`, `:7681`, and `:7682` on a neighbour's
address and get a shell or an IDE as that workspace's `dev` user. `dev` has `NOPASSWD:ALL` inside
the container (`deploy/image/build-image.sh`). The container is unprivileged on the guest, so this
is not a host escape. It is a break of the boundary between two devspaces that share a tenant VM.

`deploy/tests/ws-port-isolation.sh` checks the other-tenant case only.

Not changed here. A connection token that the edge does not know, or a bind to localhost, would lock
out the current edge path. The fix is an inner firewall (only the VM's uplink / the edge source may
reach those ports) or a localhost bind plus a proxy that is the only thing on the bridge. That needs
a live test on a throwaway tenant. See `docs/design/devspace-vm.md`.

### High

**H1. Listing nested instances was wrong for `pj-*` projects. Fixed.**

M1 puts each Shippin project in an inner Incus project `pj-<uuid>`. Three call sites did not see them:

- `tenantInstances` asked for `all-projects=true` and then `place.query` appended `project=ws`.
  Incus 6.0.6 returns 400 when both are set (`Cannot specify a project when requesting all projects`,
  `cmd/incusd/instances_get.go`). The handler swallowed that and reported `partial: true`, so operator
  drift checks treated the inner list as unknown.
- `deleteTenantVM` listed only project `ws`. Edge routes, previews, and ports for `pj-*` workspaces
  were left behind when the VM was force-deleted. Activation containers `u-<account>` were refused as
  unexpected names, which could abort the delete, or were invisible and leaked the `u-` route.
- `waitInner` and `innerView` used the same single-project query, so VM ready and the operator's
  nested-container list ignored most workspaces.

`place.queryAll` issues `all-projects=true` without a project parameter. Delete classifies only
`w-<uuid>` and `u-<uuid>`, removes each in the project the guest reported (`ws` or `pj-<uuid>`),
unpublishes `u-<account>`, and still refuses any other name before the VM is deleted.

**H2. A workspace could be larger than its plan. Fixed by capping, not by failing the create.**

`validTemplate` allowed 64 vCPU / 256 GiB / 2048 GiB. `innerInitArgs` applied that size whenever it
was non-zero, and used the plan only as the default. A caller could pin `limits.cpu` above
`workspace_cpu`. Create now runs the size through `capSize`: CPU and memory drop to the plan's
workspace cap, disk drops to the smaller of the tenant quota and the VM data disk. Zero still means
"use the profile". A template that asks for 4 GiB on a 3 GiB tier provisions at 3 GiB and the
progress line says the size was capped. Micro's workspace memory is 3 GiB while the template default
is 4 GiB; rejecting would have broken that tier.

**H3. Shared AI home versus isolated idmaps. Not changed.**

`ai.volume_shifted` is false in `deploy/hostd.json` and on gold. Inner instances set
`security.idmap.isolated=true`, so uid 1000 in two containers is two different host uids. A volume
attached to both is owned correctly in one and `nobody` in the other unless the pool supports
idmapped mounts. `docs/architecture/ai-subscriptions.md` already says to verify shifted mounts on
gold's ZFS before flipping the flag. Flipping it in this PR, without that check, would be a guess.
Logins in `~/.ai` are the thing that breaks.

### Medium

**M1. Host reboot depends on Incus 6.0's unset `boot.autostart`. Not forced on.**

`boot.autostart` is not set on the tenant VM or on workspaces. On the Incus 6.0.6 gold runs, an unset
key restores the last power state. The 2026-10-16 live check saw workspaces come back when the VM
was started. Current Incus docs (6.21, December 2025) say the same for unset, and add an explicit
`last-state` value that 6.0 rejects. `true` means always start, including instances the user had
stopped. This change does not set `true`.

The hazard is an Incus upgrade that treats unset as never-start. After that upgrade, set
`boot.autostart=last-state` on the tenant VM and on workspaces. Do it as part of the upgrade, not
before 6.0 will accept the value.

`waitInner` still returns success after 90 seconds if a workspace that was running has not come
back. The operator view is then the source of truth. Making that a hard error would turn a slow
boot into a failed VM start. Left as it is.

The session manager is different: tmux dies with the container. Phase 1 (flag off) puts a systemd
oneshot on the workspace so the windows are recreated on boot. Resume is the CLI's own history on
`~/.ai`, which is the model Codespaces and Gitpod use. Process memory is not snapshotted.

**M2. credwriter is installed on the first connect action, not at workspace create.**

`kickWriter` copies the binary, writes the unit, and starts it. Until that runs, a pasted token has
nowhere to go and the profile hook is absent. Installing it for every workspace would put a
long-running credential process, and a copy of the host binary, into workspaces that never connect.
Left for a connect-time install. Worth knowing when testing a fresh devspace.

**M3. Each inner `pj-*` project gets the full `limits.instances` of the plan.**

`innerRestrictions` sets `limits.instances` per project. Ten Shippin projects can each hold the
whole tenant cap. The host project cap does not apply to instances inside the VM. A real tenant-wide
cap has to be enforced by hostd (count across projects) or by a single inner project. Not a one-line
change. Called out for the tier work.

**M4. Workspace images are not reproducible.**

`deploy/image/build-image.sh` takes openvscode-server, Go, Node LTS, and ttyd from "latest" at build
time. The openvscode tarball is not checksummed. ttyd and Node are, against the checksum file
published next to that latest build. `deploy/image/build-tenant-vm.sh` pins the Zabbly key
fingerprint and the `lts-6.0` suite, not the `incus-base` version. Pinning a version in this PR
without a recorded checksum would freeze whatever GitHub returned on the day someone next runs the
script. Do that as its own change with the versions gold already published (openvscode-server
1.109.5 is what the gold probe saw in the ai1 images).

**M5. No in-place upgrade of a tenant VM, and no backups in hostd.**

`shippin-tvm-init` is pushed on each `initInner`, so the script can change without a new VM image.
The VM root image itself is never rebuilt under a running `tvm-`. Workspace rebuild re-inits the
root disk and keeps the home volume. There is no schedule for ZFS snapshots or `zfs send`. The
design in `docs/architecture/vm-first-tenants.md` section 9 is still the plan. Site publish does
snapshot a workspace for `pub-*`; that is not tenant recovery. Tenant delete keeps the ZFS dataset
on purpose.

**M6. Grok was a connect provider and not an agent provider. Fixed in config.**

`connect.providers.grok` and `ai.logout.grok` existed. `agent.providers` had claude and codex only.
Grok is now `run: [grok]`, `resume: [grok, --resume]` (xAI CLI: `--resume` with no id continues the
latest session in the working directory). The session manager that creates a window for it stays
off. See the design doc.

### Low

**L1. Inner image cache `/var/lib/shippin/images` is never pruned.** Already in the 2026-10-16 review.
The follow-up leaves that cache where it is: it is a platform template cache, not tenant data.
The copy pushed into a VM is staged on the org data volume and deleted after import.

**L2. Three plan tables disagreed.** Gold's live hostd has `free` (no `vm`) and a founder VM of
16 vCPU / 32 GiB / 300 GiB data / 450 GiB quota. This repo's `deploy/hostd.json` had
`micro` / `single` / `dual` / `b1`–`b8` and no `free`. The catalog seed had `free` / `pro` /
`team` / `enterprise`. The follow-up makes `docs/plans.md` the source of truth and points
`deploy/hostd.json` and the seed at `free`, `starter`, `pro`, `business`, and `founder`
(not sold). Gold's live file is unchanged. Sizes in the doc are proposals except the mail
numbers and the identity / Vaultix placement. Migration `014` still inserts the old ladder;
`028` marks those rows inactive.

**L3. The live console build is not in git.** `f64b96c` (founder-role) is not an object in this
repository. `approve-p1-only` at `fa18185` is the closest parent. A future console deploy from git
will not match what is serving until that delta is recovered or deliberately dropped.

**L4. Observability is pull-only.** Host stats, merged Incus metrics, and the operator VM view
exist. There is no alert path, and a failed inner list used to look like an empty tenant (H1 made
that `partial` only when the query fails). The session manager writes
`~/.shippin/sessions.json` inside the workspace. Nothing scrapes it yet.

**L5. Tests are fakes.** `go test ./internal/hostd/` covers placement, the all-projects query shape,
delete of a `pj-*` workspace, the size cap, agent argv, and the session-manager script under a fake
`tmux`. Nothing in CI boots KVM, nested Incus, or a reboot. The 2026-10-16 live notes are still the
only end-to-end evidence, and they predate `pj-*`.

## What this change fixes

- All-projects listing, VM ready wait, operator nested view, and tenant delete (H1), including
  activation-container routes.
- Plan caps on workspace CPU, memory, and disk (H2).
- Rebuild validates the template before it stops the workspace.
- Grok as an agent provider, with the same argv checks as the other CLIs (M6).
- Extra permission-bypass flags rejected in agent config.
- Phase 1 session manager, default off (`agent.session_manager`). Described in
  `docs/design/devspace-vm.md`. Shipping the flag false means a config that omits the key, including
  gold's current file, keeps today's single tmux session.

## What this change does not do

- No deploy, no image rebuild, no edit of gold's hostd.json, no change to a running VM.
- No inner firewall for C1.
- No flip of `volume_shifted`. The gold ZFS probe is written down; the flag stays false.
- No snapshot schedule. Publish snapshots stay on the instance disk, which is the org pool or
  the inner btrfs.
- No Claude Desktop, Codex desktop, or Grok desktop install, and no certificate design. Placement
  of the one install (control plane or tenant VM) is open in the design doc.
- No running `hub-server`. The per-org design is written against `shippinAI/agent-hub`; it is not
  wired into hostd.
- No `boot.autostart=true`.

## Follow-up: per-org storage

Git publish and the site scanner wrote tenant trees to the host temp directory. Import staging
used the VM root. A plan change did not raise an existing dataset quota or the VM disks. Those
three are fixed. `POST /tenants/{id}/storage`, `POST /tenants/{id}/plan`, and
`shippin-hostd storage add` grow the quota, the root, the data volume, and the inner btrfs, and
refuse a shrink. `low_space` is set on the storage and VM views. Details and the list of bytes
that stay shared on purpose (template cache, starter Postgres, gitleaks) are in
`docs/architecture/org-storage.md`.
