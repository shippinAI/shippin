# Devspace VM

Status: target architecture, 2026-10-22. Decision owner: the operator Hall.
Phase 1 is implemented behind `agent.session_manager` (default **false**) and is not deployed.
The review of the code this design sits on is `docs/REVIEW-devspace-2026-10.md`.

## 1. Decision

Each devspace is a full VM, with containers inside it, so nested KVM can work for Cowork once a
desktop placement is chosen. The user gets Claude Code, Codex, and Grok in one terminal session
manager, reached from the browser through the console and the edge. A lovable-style builder uses
the user's own subscriptions. Shared task state is `shippinAI/agent-hub` (section 6), one hub per
org, not a new repo and not a new protocol.

That is the target. It is not what the reference host runs, and it is not what phase 1 builds.

Claude Desktop, Codex desktop, and Grok desktop are **not installed** by this work and no placement
is chosen. the operator wants one install of each, serving either the control plane or the tenant VM
(shared by that org's workspaces). That choice is still being researched. This document keeps both
open and does not design TLS certificates for those apps. SSH certificates for local Linux accounts
are specified separately in `docs/design/identity-local-accounts.md` and are not built here.

What the reference host runs today:

- One KVM VM per **tenant** (`tvm-<uuid>`), when the plan has a `vm` block and the tenant is
  `user.shippin.mode=vm`.
- Devspaces are unprivileged Incus containers **inside** that VM (`w-<uuid>` in `pj-<uuid>`).
- The free plan on the reference host has no `vm` block, so a free tenant gets a container on the host and no
  `/dev/kvm`.
- Claude Code, Codex, and the Grok CLI are in the workspace image (`shippin/dev-debian13-ai1`),
  not in the tenant VM. The VM image has Incus and no tmux, node, or CLIs (host probe, 2026-10-22).
- One tmux session named `agent` is started on demand for one provider. Grok was not a provider.
- Browser terminal is ttyd on `/_term/`, behind the same edge sign-in as the IDE.

Phase 1 puts the session manager in the workspace container, which is the instance that already
has the CLIs. When a later phase makes the devspace itself the VM, the same unit moves with it.
Installing tmux in the tenant VM image now would boot empty windows.

## 2. Placement by tier

Names and numbers live in `docs/plans.md`. Hostd, `deploy/hostd.json`, and the catalog seed match
that file. The reference host's live hostd does not: it still has `free` and `founder` only. Do not deploy this
file to the reference host.

| Tier | Devspace | KVM in the tenant VM | Cowork room (proposal) |
|---|---|---|---|
| free | Containers on pool `p-<tenant>`. No VM. | No | No |
| starter | Per-org VM, 2 vCPU / 4 GiB, data 40 GiB | Yes | No. 4 GiB of RAM is under the 8 GiB the desktop documents. |
| pro | Per-org VM, 4 vCPU / 16 GiB, data 100 GiB | Yes | Yes, if the desktop placement is the tenant VM. Data volume has the 25 GiB. |
| business | Per-org VM, 8 vCPU / 32 GiB, data 280 GiB | Yes | Yes, same condition |
| founder | Per-org VM, 16 vCPU / 32 GiB, data 300 GiB, quota 450 GiB. Not sold. | Yes | Yes, same condition |

Those sizes are proposals. The structural rules (free is a container, starter and up are a VM,
Cowork only where RAM and the data volume allow) are the ones the operator already set.

What each desktop placement would need, when someone picks it. Neither is built.

| | Control plane, one install for the platform | Tenant VM, one install per org |
|---|---|---|
| Binary | Platform disk. Not copied into every workspace. | Once inside `tvm-<tenant>`, on the data volume, shared by that org's workspaces. Not once per workspace. |
| Cowork's ~25 GiB disk | A file or volume on `tank/tenants/<tenant>`, mounted into the session. Not a platform pool, not the 20 GiB VM root. | A volume on the inner btrfs (`tvd-<tenant>` / pool `p-<tenant>`). Same rule. |
| RAM | The control-plane session needs about 8 GiB, and about 4 GiB while Cowork runs. The tenant VM's own RAM floor does not have to be 8 GiB for Cowork. | The plan's VM needs at least 8 GiB. `starter` does not. `pro` and above do, in the proposal. |
| Devices | The machine that runs QEMU needs `/dev/kvm` and `/dev/vhost-vsock`. | Those devices on the tenant VM. Inner containers stay `limits.virtual-machines=0` and do not get `unix-char`. |
| Plans | `cowork: true` still means the org data volume must keep 25 GiB free. The low-space signal already says so. | Same, plus the VM memory check in `Config.Check`. |
| Certificates | Not part of this design. | Not part of this design. |

Until a later phase, paid tenants keep one VM per org and containers per devspace. Free stays a
container. Phase 1 does not move them and does not install a desktop.

## 3. Session manager

Choice for phase 1: **tmux, one session, one window per provider, ttyd as the browser attach.**

| Option | Why it is or isn't phase 1 |
|---|---|
| tmux | Already in every workspace image. The agent wrapper and both ttyd units already speak it. Windows are the multiple sessions. No new binary. |
| zellij | Better layouts. Not installed. Another unpinned download on top of an image build that already tracks "latest". |
| abduco / dtach | Survives a disconnect, not a container stop, and has no windows. |
| A purpose-built web terminal | ttyd is already that attach point. Replacing it means a new auth path next to `forward_auth`. Later, if the product needs tabs that are not tmux keystrokes. |

What "persistent" and "resumable" mean, the same way Codespaces, Gitpod, and Coder mean them:

- The container's disk and the AI home survive stop and host reboot. Vendor conversation state
  lives there (`CLAUDE_CONFIG_DIR`, `CODEX_HOME`, `GROK_HOME` under `~/.ai`).
- The tmux server does not survive a stop. A systemd oneshot (`shippin-sessions.service`) recreates
  the session and a window per provider on boot. Windows start as a login shell. The CLI is not
  launched until something asks, so a reboot does not spend the user's subscription.
- Resume is the vendor flag, run in that window: `claude --continue`, `codex resume --last`,
  `grok --resume` (no id: latest session in the working directory, per xAI's CLI docs).
- The browser attaches with the existing ttyd unit: `tmux new -A -s agent`, path `/_term/`,
  edge `forward_auth`. The user switches windows inside the session.

Flag: `agent.session_manager` in hostd.json. Default false, including the file in this repo.
The reference host's live file has no such key; the zero value is false, so this binary does not change a
running workspace until the key is set and hostd is restarted. When it is true, hostd installs
the script and the unit at workspace create, start, restart, and agent start. The unit is what
runs after a reboot, without hostd in the path.

A window that is already running `shippin-agent` or the CLI is left alone. Resume does not kill
a live agent.

## 4. Claude Desktop and Cowork on Linux

Checked against Anthropic's docs on 2026-10-22:

- [Claude Desktop on Linux (beta)](https://code.claude.com/docs/en/desktop-linux)
- [Install Claude Desktop](https://support.claude.com/en/articles/10065433-install-claude-desktop)
- [Download Claude](https://claude.com/download)

There **is** an official Linux build. It is beta. It is not "macOS and Windows only".

| | What Anthropic says |
|---|---|
| Distros | Ubuntu 22.04+ or Debian 12+, x86_64 or arm64. Other Debian-family distros may work and are not tested. Fedora and Arch are told to use the CLI. |
| Install | apt repo `https://downloads.claude.ai/claude-desktop/apt/stable`, package `claude-desktop`. Key fingerprint `31DD DE24 DDFA B679 F42D 7BD2 BAA9 29FF 1A7E CACE`. Updates come from apt, not from the app. |
| Plans | Chat on every plan, including Free. Claude Code and Cowork in the desktop app on Pro, Max, Team, and Enterprise. |
| Not on Linux | Computer use. Dictation. Quick Entry on native Wayland needs the desktop's GlobalShortcuts portal. |
| Cowork | Runs its tasks in a VM the app hosts with QEMU and KVM. Needs hardware virtualization, `qemu-system-x86`, `ovmf`, `virtiofsd` (x86_64), the user in group `kvm`, access to `/dev/kvm` and `/dev/vhost-vsock`, about 25 GiB of disk for the workspace image, and at least 8 GiB of RAM. The workspace uses 4 GiB while it runs. |

Debian 13 meets the written "Debian 12 or later" bar. Anthropic does not name 13 as a tested
release. Do not bake the package into an image until it has been installed on a throwaway Debian
13 VM.

The reference host's host has nested KVM (`kvm_intel nested=Y`). The running tenant VM has `/dev/kvm`. The probe
did not check `/dev/vhost-vsock` or that `dev` is in group `kvm`. Both are Cowork requirements.
That check is a host action, not a code change.

Cowork cannot run in a workspace container. Inner projects set `limits.virtual-machines=0`,
`restricted.containers.nesting=block`, and `restricted.devices.unix-char=block`. The container
cannot open `/dev/kvm` or run a VM. If the one desktop install is placed in the tenant VM, QEMU
runs there, once, shared by that org's workspaces, with `/dev/kvm` and `/dev/vhost-vsock` on that
guest. If it is placed on the control plane, QEMU runs there and the 25 GiB disk is still the
org's data volume, mounted in. Nested containers stay for untrusted builds either way and do not
get those devices.

Phase 1 does not install Claude Desktop, Codex desktop, or Grok desktop. The CLIs are the path
that works today. No certificate design for a desktop session.

## 5. Browser access to GUI apps

The tenant VM has no desktop. The console reaches the IDE and ttyd over HTTP through the edge.
A GUI needs a display and a browser path. Honest options, none of them chosen:

1. A per-VM web desktop (Selkies, KasmVNC, or noVNC) published as another edge route behind the
   same `forward_auth` as the IDE. The port stays off the public internet and off the inner
   bridge's free-for-all (review finding C1). This is the usual shape for a Linux cloud desktop.
2. Do not offer the GUI. Claude Code, Codex, and Grok stay in the session manager. Cowork waits
   until the desktop path exists. This is phase 1.
3. Chrome Remote Desktop or a vendor-hosted stream. Extra account, harder to put behind Nomen,
   not recommended.

VNC must not listen on the tenant network without the edge in front. The IDE's missing connection
token is already a hole inside the VM (C1). A GUI would be a worse one.

## 6. Agent hub

The hub already exists: `github.com/shippinAI/agent-hub` (Go). This repo does not fork it and does
not add a second protocol. Phase 1 does not run it.

What that repo is, from `README.md`, `docs/spec.md`, and `docs/connect.md` on `master`:

- `server/cmd/hub-server` is one process. It serves the UI, REST `/api/v1`, MCP `POST /mcp`
  (Streamable HTTP), and SSE `/api/v1/stream`. Its only durable store is one Postgres database
  (`HUB_DATABASE_URL_FILE`). Tokens are stored as SHA-256. Events are append-only and hash-chained.
- A task is claimed under a lease with a fence (`409 stale_fence` / `not_holder` / `lease_expired`
  means stop writing). Handoff is a note plus release in one transaction. Presets (agents, roles,
  policies, workflow) are rows, seeded from `server/internal/presets/default.json`.
- Projects are rows (`projects(id, name, workflow_id)`). There is no tenant or org column on
  tasks. One database is one trust boundary.
- Agents connect with a bearer token (`ahk_…`) per the snippets in `docs/connect.md`: Claude Code
  `.mcp.json`, Codex `bearer_token_env_var`, Grok `grok mcp add`. Humans use `HUB_HUMAN_AUTH`
  (`proxy-header` is the mode that sits behind an identity proxy; `dev` is loopback only). The
  spec names oauth2-proxy in front of Zitadel as an example of `proxy-header`. Nomen is our
  Zitadel fork, so that is the existing hook. This design does not add a certificate.
- `bridge/` is the machine bridge (files, processes, policy gate). It is not required to put a
  hub database on an org's disk.

Per-org shape, on top of that binary:

- One `hub-server` per org, shared by that org's workspaces. Not one server per workspace, and
  not one shared `agenthub` database for every customer. A shared database would mix orgs, and
  the schema has nowhere to put an org id.
- The Postgres data directory is a volume on `p-<tenant>` (legacy `free`) or on the tenant VM's
  data disk (starter and up). Same rule as every other tenant byte. It does not use the platform
  starter cluster. That cluster stays the Vaultix free/starter exception only.
- A devspace is a hub **project** row inside the org's database. Workspaces in the org point
  their CLIs at that one `HUB/mcp`, with a per-agent token file on the org's `~/.ai` (mode `0600`),
  not the user's Nomen access token. `internal/config/agents.go` still stands for the console's
  own agent identity (IdP service user now, Nomen token exchange with `act` later). The hub token
  is the hub's token, minted by `hub-server admin token create`, stored on the org dataset.
- Human UI auth is `HUB_HUMAN_AUTH=proxy-header` behind the edge the console already has. No new
  certificate, and no `dev` auth off loopback.
- `~/.ai/shared/` remains a scratch directory on the AI home for files the hub does not own.
  Task state, leases, notes, and the audit chain are the hub's Postgres, on the org volume.
- The session manager does not start the hub and does not start desktop apps. The hub does not
  start the CLIs. `docs/connect.md` is how a CLI attaches.

Where the process listens follows the desktop placement, which is open:

- Tenant VM: `hub-server` runs once inside `tvm-<tenant>`, bound to the inner bridge, reachable
  by that org's workspaces and not by the next tenant. The database files are on the data volume.
- Control plane: one `hub-server` process per org still, but the process is scheduled on the
  platform and its data directory is a dataset mount of `tank/tenants/<tenant>`, not a directory
  on the platform pool. Workspaces reach it through the edge, with the same bearer token. The
  binary is platform. The bytes are the org's.

Neither placement is implemented in hostd.

## 7. Lovable-style builder

Not in phase 1. The pieces it needs are already the product, used by the user rather than by a
hosted model:

- Prompt in, files out: `PUT …/prompt` and `POST …/agent` with `provider` of `claude`, `codex`,
  or `grok`. The prompt travels on stdin. The CLI runs as `dev` with that user's `~/.ai`.
- Preview: the existing preview routes.
- Sign-in: credwriter and the vendor CLIs. Shippin does not hold an API key for this.

The builder UI is a console surface on top of those routes. It should not grow a second agent
runner. Phase 1 makes the third provider and the resumable windows exist so that surface has
something reliable to call.

## 8. Subscription login

Today, on the reference host: hostd connect plus `shippin-credwriter` writes vendor CLI credentials into
`~/.ai` (mode `0600`, env file sourced by login shells). Claude, Codex, Grok, and GitHub are
connect providers. Vendor logout argv is in config. The activation container `u-<account>` is
ttyd plus tmux for the browser login. chatmaton (private repo `example-org/chatmaton`) is the
intended chat engine and is not deployed. `ai-launcher` was installed by hand in one devspace.

Phase 1 does not replace credwriter and does not vendor chatmaton. The session manager assumes
the CLIs are already signed in on the AI home. A window that is not signed in is the vendor's
own login prompt, in that window, which is the same rule as the activation terminal: the sign-in
is between the user and the vendor.

## 9. Phases

Each phase is shippable on its own. Later phases are not started because the flag exists.

| Phase | What | Gate |
|---|---|---|
| 0, this PR | Review, the listing and size fixes, this document. | Merged. Not deployed. |
| 1, this PR, flag off | Session manager unit. Windows for every `agent.providers` entry. Grok added. Tests, including the boot script against a fake tmux. | the operator sets `agent.session_manager` true on a throwaway tenant. Not the reference host's founder VM. |
| 2 | Devspace is the VM. New paid devspaces get `dvm-<id>` instead of a container in the tenant VM. Free stays a container. Image carries the CLIs, tmux, and the session unit. Migration of existing containers is a separate, explicit step. | C1 has a fix (inner firewall or equivalent) before two customers share a bridge. `volume_shiftedthe removed single-site signing code docs/architecture/org-storage.md`). |
| 3 | One install of Claude Desktop, Codex desktop, and Grok desktop, after the operator picks control plane or tenant VM. Cowork's disk on the org data volume. No install before that, and no certificate work in that change. | Placement decided. `/dev/kvm` and `/dev/vhost-vsock` confirmed on the machine that will run QEMU. Apt key checked the way Zabbly's key is checked, if the install is apt. |
| 4 | One `hub-server` per org from `shippinAI/agent-hub`, database on that org's dataset, MCP as in `docs/connect.md`. | Phase 1 flag has been on for a real workspace without the session unit failing closed. Desktop placement does not block this; the hub does not install a desktop. |
| 5 | Builder UI on the existing agent routes. | Phase 1 providers, including Grok, resume from the UI. |

## 10. Decisions for the operator

1. Confirm or replace the proposed CPU, RAM, disk, workspace, boundary, and price columns in
   `docs/plans.md`. Mail numbers, identity placement, and Vaultix shared-vs-dedicated are already
   his. The test will fail until the JSON block, `deploy/hostd.json`, and the catalog seed move
   together.
2. Where the one desktop install sits: control plane, or one copy inside the tenant VM. Section 2
   lists what each needs from storage and plans. Do not install either to find out, and do not
   design certificates for it.
3. When phase 2 starts. Until then, paid customers share one tenant VM and are exposed to C1.
4. `volume_shiftedthe removed single-site signing code docs/architecture/org-storage.md`, before
   any second workspace mounts the same AI home.
5. Whether `/dev/vhost-vsock` exists on the machine that would run Cowork, and whether the desktop
   user is in group `kvm`. Do not install the package to find out.
6. Incus upgrade. Stay on 6.0 until `boot.autostart=last-state` is the explicit key (6.21+).
   Do not set `boot.autostart=true`.
7. Backup target and retention. Still unimplemented. The dataset is kept on tenant delete; that
   is not a backup. Snapshots that do exist are on the org dataset.
8. The live console build `f64b96c` is not in git. Recover it or accept that the next console
   deploy from `approve-p1-only` drops whatever that build added.
9. Turn the session-manager flag on only for a throwaway tenant, after this PR is deployed.
   The reference host's founder VM is the wrong first boot.
