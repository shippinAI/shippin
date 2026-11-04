# Proxmox VE, full coverage

The console is the product. Proxmox VE 8 and 9 are a driver behind the same guest, storage, network, and task model as Incus. This document is the public-product surface: every endpoint group in the PVE API viewer (`/api2/json`, the API viewer shipped with pve-docs), who may call it, and how a call is proved. Phase 1 is implemented. Later phases are specified here and have no routes, pages, or client methods.

Official shape used below is the PVE 8 API, which PVE 9 still serves under `/api2/json`. A method that appears only in one major is called out. Paths are relative to `/api2/json`.

## Rules for every call

**Version.** `GET /version` returns `version`, `release`, and `repoid`. The driver records that string on health (`internal/hv/proxmox` `Health`). A major other than 8 or 9 is an error, not a guessed dialect. Phase 1 stores the version string and does not yet branch on it, because the phase 1 paths are the same in 8 and 9.

**Cluster or one node.** `GET /cluster/resources` works on a single node (a cluster of one) and on a real cluster. Guest inventory always comes from that list, then the per-guest call uses the node the resource row names. The configured node (`servers.hv_node`) is the node that receives create, upload, vzdump, and the other node-local calls. It is not discovered from a name compiled into the binary. If that node is absent from `GET /nodes`, the call fails.

**TLS.** The client verifies TLS. Trust is the system pool plus an optional PEM file (`servers.hv_ca_file`). Minimum version is TLS 1.2. There is no skip flag.

**Auth.** `Authorization: PVEAPIToken=<token_id>=<secret>`. The token id is a column. The secret is the process environment variable named by `servers.hv_token_env`. The client does not use the password ticket API (`POST /access/ticket`) and does not store a root password. A 401 is an error to the caller.

**Tasks.** Mutations that return a `UPID:` string are polled at `GET /nodes/{node}/tasks/{upid}/status` with the log at `.../log`. The wait starts at 200ms and doubles to 5s. A stopped task whose `exitstatus` is not `OK` is an error. The log lines are returned with the task. `GET /v1/infra/servers/{id}/tasks/{upid}` is one read. Create, clone, template, delete, resize, migrate, snapshot, and rollback wait, then read the guest again. Download, upload, and vzdump return the task id as 202; the page polls the task route.

**Read-back.** A lifecycle call that finds the guest already in the requested state does not POST. After a POST it GETs inventory and returns that body. If the status is not the one requested, the error includes the status the API reported. Delete waits, then requires the id to be absent.

**Guest placement.** Creating a guest requires a storage selected from the attached node's reported storage and suitable for that guest kind (`rootdir` for LXC, `images` for QEMU), a Linux or Open vSwitch bridge selected from the node's reported network interfaces, and a positive disk size. The panel sends those choices; the driver checks them against the provider again before it creates anything. It never selects the first storage or network and never inserts a disk-size default. Incus keeps its own instance image and project behavior and does not receive these Proxmox-only placement fields.

**Who may call.** `/v1/infra/servers` requires `roles.infra` (`op_infra` unless config says otherwise). Platform admin is not implied. Reads stop there. Mutations also pass `hv.Guard` with action `hv.lifecycle`. There is no shipped approver rule now: an empty approval policy denies lifecycle mutations. AgentHub policy and hash-bound ticket review are not integrated yet; see [configurable human review](../approval.md). Do not treat the local Shippin task board as AgentHub.

**Audit.** Each permitted mutation writes one audit row, action `hv.lifecycle`, detail `<server id> <verb> <guest id>`. The token id, the secret, and the API URL are not in the detail. If the audit write fails, the hypervisor is not called.

**Rate limits.** Not implemented in phase 1. Phase 2 adds a per-server token bucket in the client: 10 requests per second, `Retry-After` and HTTP 429 double the wait, and the same error string does not get a new log line (the reconciler rule in `internal/hv`). No limiter is mounted half-finished.

**Tenants.** A tenant lists and operates guests only on a server it owns or is assigned (`server_assignments`). Estate rows with no assignment stay on the operator routes. Create, delete, migrate, upload, and attaching the API token stay operator-only. Everyday power and snapshots are on `/v1/tenants/{tenantId}/servers/{serverId}/...` for an org owner or admin, with the same lifecycle policy gate. The sample policy is empty and therefore denies these mutations until AgentHub review is integrated. Pool ACL (`/pools/{org}`) is still phase 4: until then the server assignment is the isolation boundary, and the console does not show another org's guests.

## Phase plan

| Phase | What | Code |
|---|---|---|
| 1 | Health and version, guest inventory with provider-reported CPU, memory, disk, network counters, uptime, pool, template and tags, qemu and lxc create/clone/template/delete/config/resize/migrate, start/stop/shutdown/reboot, snapshots and rollback, storage list and content, ISO/template upload and download-url, node services and network read, firewall read, vzdump, backup content list, restore from an archive, UPID status and log. Tenants get read, power, and snapshots on servers they own or are assigned | `internal/hv/proxmox`, `/v1/infra/servers`, `/v1/tenants/{tenantId}/servers/{serverId}`, `/operator/hosts`, `/t/{tenantId}/servers/{serverId}` |
| 2 | Node disks (directory, LVM, LVM-thin, ZFS), apt read, certificates read, DNS and time read, syslog and journal read, task list, firewall write, cluster backup job list, pools read, HA read, replication read | Physical disk inventory via `/nodes/{node}/disks/list?skipsmart=1` is authored in `internal/hv/proxmox` and shown in the existing `web/` node detail view. It is best-effort and read-only; denied inventory leaves other node facts available. Existing node storage-pool capacity is also shown in the provider view. Additional disk initialization/configuration, pool-specific storage inventory and every other phase 2 item remain not started. |
| 3 | SDN zones/vnets/subnets, Ceph, ACME write, subscription read, notification targets, metrics servers, wake-on-lan, cloud-init, guest agent exec and file, termproxy/vncproxy/spiceproxy, move-disk, prune, cluster options | Not started |
| 4 | Pools and ACL per org, tenant-exposed guests, PBS native verify/prune/sync. PVE users, groups, realms, tokens, and TFA are not copied into a console directory | Not started |

## Endpoint groups

Exposure is `operator` (infra role) or `tenant` (org member, only their pool). Approval is blank for reads. Async is "UPID" when the PVE method returns a task id.

| Group | PVE paths | Phase | Exposure | Isolation | Approval verb | Async | Read-back |
|---|---|---|---|---|---|---|---|
| Version | `GET /version` | 1 | operator | none | | | the version string |
| Cluster status | `GET /cluster/status`, `GET /nodes` | 2 | operator | none | | | |
| Resources | `GET /cluster/resources` | 1 | operator; tenant in phase 4 | phase 4: pool = org | | | this list is the inventory |
| HA | `GET/POST /cluster/ha/groups`, `/cluster/ha/resources`, `GET /cluster/ha/status` | 2 read, 3 write | operator | groups are not a tenant boundary; pools are | `ha` | UPID on changes | status current |
| Replication | `/cluster/replication`, `/nodes/{node}/replication` | 2 read, 3 write | operator | job id scoped to the guest's pool in phase 4 | `replication` | UPID | job config GET |
| Backup jobs | `/cluster/backup`, `/cluster/backup/{id}` | 2 | operator | job's storage must be the org's in phase 4 | `backup-job` | | job GET |
| Cluster firewall | `/cluster/firewall/options`, `groups`, `rules`, `aliases`, `ipset` | 2 read, 3 write | operator | cluster-wide; never tenant-writable | `firewall` | | rules GET |
| SDN | `/cluster/sdn/zones`, `vnets`, `subnets`, `controllers`, `dns`, `ipams` | 3 | operator | a vnet may be named per org; not assumed | `sdn` | apply is UPID | `GET` after apply |
| Cluster options | `GET/PUT /cluster/options` | 3 | operator | operator-only | `cluster` | | options GET |
| Notifications | `/cluster/notifications/targets`, `matchers`, `endpoints` (PVE 8.1+) | 3 | operator | operator-only | `notify` | | target GET |
| Metrics | `/cluster/metrics/server` | 3 | operator | operator-only | `metrics` | | server GET |
| Cluster tasks | `GET /cluster/tasks` | 2 | operator | phase 4 filters to the org's guests | | | |
| Node status | `GET /nodes/{node}/status` | 1 via health's node; full status 2 | operator | the node of the attached server | | | |
| Node services | `GET /nodes/{node}/services`, `POST .../services/{service}/{start,stop,restart,reload}` | 1 read; write 2 | operator | operator-only | `service` | UPID | service state |
| Node network | `GET/POST /nodes/{node}/network`, `DELETE/PUT .../network/{iface}` (bridge, bond, vlan, OVS) | 1 read; write 3 | operator | operator-only | `network` | reload is UPID | iface GET |
| DNS and time | `/nodes/{node}/dns`, `/nodes/{node}/time` | 2 | operator | operator-only | `node` | | GET |
| Disks | `/nodes/{node}/disks/list`, `directory`, `lvm`, `lvmthin`, `zfs`, `zfs/{name}` | 2 | operator | operator-only | `disk` | create is UPID | list GET |
| Apt | `/nodes/{node}/apt/update`, `changelog`, `versions`, `repositories` | 2 read; upgrade 3 | operator | operator-only | `apt` | upgrade is UPID | versions GET |
| Certificates | `/nodes/{node}/certificates/info`, `custom`, `acme`, `acme/certificate` | 2 read; ACME 3 | operator | operator-only | `cert` | order is UPID | info GET |
| Subscription | `GET/POST/DELETE /nodes/{node}/subscription` | 3 | operator | operator-only | `subscription` | | GET |
| Syslog | `/nodes/{node}/syslog`, `/nodes/{node}/journal` | 2 | operator | operator-only | | | |
| Tasks | `/nodes/{node}/tasks`, `/tasks/{upid}/status`, `/tasks/{upid}/log` | 1 status and log; list 2 | operator | | | poll | exit status |
| Consoles | `termproxy`, `vncproxy`, `vncshell`, `spiceproxy` on the node and on qemu/lxc | 3 | operator | a tenant console waits until pools exist, then only that org's guest | `console` | proxy ticket, short lived | not a guest status |
| Wake-on-LAN | `POST /nodes/{node}/wakeonlan` | 3 | operator | operator-only | `power` | | a later status GET, which may fail until the node answers |
| QEMU create | `POST /nodes/{node}/qemu` | 1 | operator; tenant in phase 4 | new guest's pool is the org; storage and bridge selected from this node's live inventory | `create` | UPID | guest in resources |
| QEMU clone | `POST /nodes/{node}/qemu/{vmid}/clone` | 1 | operator; tenant in phase 4 | clone stays in the source pool | `clone` | UPID | new id in resources |
| QEMU template | `POST /nodes/{node}/qemu/{vmid}/template` | 1 | operator | templates are operator-owned until phase 4 names an org | `template` | UPID | guest tags or template flag from config |
| QEMU delete | `DELETE /nodes/{node}/qemu/{vmid}` | 1 | operator; tenant in phase 4 | only the org's guest | `delete` | UPID | id absent |
| QEMU config | `GET/PUT /nodes/{node}/qemu/{vmid}/config`, `POST .../config` (sync) | 1 | operator; tenant in phase 4 for its guests | pool unchanged by a tenant PUT | `config` | PUT may be null or UPID; the client treats a null body as sync | config GET |
| QEMU migrate | `POST /nodes/{node}/qemu/{vmid}/migrate` (`online` for live) | 1 | operator | target node must be in the same attached cluster | `migrate` | UPID | resource row's node |
| QEMU resize | `PUT /nodes/{node}/qemu/{vmid}/resize` | 1 | operator; tenant in phase 4 | org guest | `resize` | UPID | config GET of the disk key is phase 2; phase 1 returns the guest row |
| QEMU move disk | `POST /nodes/{node}/qemu/{vmid}/move_disk` | 3 | operator | org guest in phase 4 | `move-disk` | UPID | config GET |
| QEMU snapshots | `GET/POST /nodes/{node}/qemu/{vmid}/snapshot`, `DELETE/GET .../snapshot/{name}`, `POST .../rollback` | 1 list, create, rollback; delete 2 | operator; tenant in phase 4 | org guest | `snapshot`, `rollback` | UPID | snapshot list contains the name; rollback returns the guest |
| Cloud-init | config keys `cicustom`, `ipconfigN`, `nameserver`, `searchdomain`, `sshkeys`, `ciuser`, `citype` | 3 | operator; tenant in phase 4 | org guest | `config` | with the config write | config GET |
| Guest agent | `POST /nodes/{node}/qemu/{vmid}/agent`, `agent/exec`, `agent/file-read`, `agent/file-write`, `agent/exec-status` | 3 | operator | org guest in phase 4; exec is approval `exec` | `exec` | exec-status poll, not only UPID | exec-status exited |
| QEMU firewall | `/nodes/{node}/qemu/{vmid}/firewall/rules`, `options`, `aliases`, `ipset` | 1 read of rules; write 2 | operator; tenant read in phase 4 | org guest | `firewall` | | rules GET |
| QEMU console and rrd | `vncproxy`, `termproxy`, `spiceproxy`, `rrd`, `rrddata`, `status/current` | 3 (rrd 2) | operator | org guest in phase 4 | `console` for proxies | ticket | |
| QEMU power | `status/start`, `stop`, `shutdown`, `reboot`, `reset`, `suspend`, `resume` | 1 start/stop/shutdown/reboot; reset/suspend/resume 2 | operator; tenant in phase 4 | org guest | the verb | UPID, then inventory | status string |
| LXC create | `POST /nodes/{node}/lxc` (`ostemplate`) | 1 | operator; tenant in phase 4 | pool = org; rootdir storage and bridge selected from this node's live inventory | `create` | UPID | guest in resources |
| LXC clone, template, delete, config, migrate, resize | same paths under `/lxc/{vmid}` | 1 | same as QEMU | same as QEMU | same verbs | UPID | same |
| LXC snapshots and rollback | `/lxc/{vmid}/snapshot` | 1 | same as QEMU | same | `snapshot`, `rollback` | UPID | snapshot list |
| LXC mount points | config `mpN`, `unusedN`; `move_volume` | 3 | operator | org guest | `config` | move is UPID | config GET |
| LXC firewall and console | `firewall/rules`, `vncproxy`, `termproxy` | 1 firewall read; console 3 | operator | org guest in phase 4 | `firewall`, `console` | | rules GET |
| Storage config | `GET /nodes/{node}/storage`, `GET/POST /storage`, `PUT/DELETE /storage/{storage}` | 1 node list; cluster config 2 | operator | a tenant sees storages linked to their pool in phase 4 | `storage` | | storage GET |
| Storage content | `GET /nodes/{node}/storage/{storage}/content`, `GET/DELETE .../content/{volume}` | 1 list; delete 2 | operator | volume's guest, when it has one, must be in the org pool | `content` | delete is UPID | list |
| Upload | `POST .../storage/{storage}/upload` | 1, 32 MiB on the console route | operator | operator-only until phase 4 names an org storage | `upload` | UPID | content list is the caller's follow-up |
| Download URL | `POST .../storage/{storage}/download-url` | 1 | operator | https URL only; the console rejects any other scheme | `download` | UPID | content list |
| Prune | vzdump `prune-backups` and PBS prune | 3 | operator | org storage in phase 4 | `prune` | UPID | content list loses the pruned volumes |
| vzdump | `POST /nodes/{node}/vzdump` | 1 | operator | guest must be visible | `backup` | UPID returned immediately | backup content list |
| Access | `/access/users`, `groups`, `roles`, `acl`, `domains`, `users/{userid}/token`, `tfa` | 4, and only as a projection | operator | Shippin roles and Nomen are the directory. The console does not create PVE users for tenants | `acl` | | ACL GET shows the projected pool |
| Pools | `/pools`, `/pools/{poolid}` | 2 read, 4 write | operator; membership is the tenant boundary | one pool per org id | `pool` | | pool GET lists the guest |
| Ceph | `/nodes/{node}/ceph/status`, `osd`, `mon`, `mgr`, `mds`, `fs`, `pools` | 3 | operator | operator-only | `ceph` | many are UPID | status GET |
| PBS via PVE | storage type `pbs`; content `backup`; create with `archive=` | 1 list and restore | operator | the PBS storage is operator-configured | `restore` | UPID | new guest in resources |
| PBS native | datastores, snapshots, verify, prune, sync, tasks on the backup server API (`/api2/json` on port 8007) | 4 | operator | a datastore per org when tenant backups exist | `pbs` | UPID on the backup server | datastore snapshot list |

`reset`, `suspend`, and `resume` are real PVE qemu verbs. They are not phase 1 routes. LXC has no suspend. The phase 1 verb set is start, stop, shutdown, reboot.

## Incus parity

One guest table. Driver-specific fields stay off that table until a later phase adds them as extras, not as a second page model.

| Console | Proxmox | Incus |
|---|---|---|
| `Guest.id` | `vmid` as a string | instance name |
| `Guest.kind` `vm` or `ct` | `type` `qemu` or `lxc` | `virtual-machine` or `container` |
| `Guest.status` | `status` copied (`running`, `stopped`, …) | `status` copied |
| `Guest.node` | resource `node` | cluster member, when Incus is clustered; otherwise the server |
| Start, stop, shutdown, reboot | `POST .../status/{verb}` | `PUT /1.0/instances/{name}/state` `{"action":...}` |
| Snapshot and rollback | `snapshot` and `snapshot/{name}/rollback` for qemu and lxc | `POST /1.0/instances/{name}/snapshots`, `PUT .../snapshots/{name}` restore |
| Template | `POST .../template` | publish an image (`POST /1.0/images`). Incus extra, not a fake template flag |
| Storage | storage id and content volid | storage pool and volume |
| Network | iface type `bridge`, `bond`, `vlan`, … | network name and type |
| Migrate | `migrate` with `target` and `online` | cluster move. Not on the Incus driver yet |
| Task | UPID status and log | operation `/1.0/operations/{id}` (Incus extra; the page's task card is the PVE shape in phase 1) |
| Firewall | rule list | instance devices and network ACLs (Incus extra, not wired) |
| No hypervisor | kind `bare` returns `ErrNoHypervisor` | not applicable |

The Instances page remains the hostd read of Incus. Hosts is the phase 1 guest page. They use the same columns (name, id, kind, status, node). Folding Incus into Hosts is phase 2 of the UI, after the Incus driver grows the phase 1 methods. That merge is not stubbed.

## Where phase 1 lives

| Piece | Path |
|---|---|
| Client, UPID wait, qemu and lxc mutations | `internal/hv/proxmox/phase1.go`, `driver.go` |
| Recorded-shape tests | `internal/hv/proxmox/phase1_test.go` |
| Server columns | `internal/db/migrations/036_server_hypervisor.sql`, `internal/servers/endpoint.go` |
| HTTP | `internal/api/infra.go` |
| Page | `web/src/features/hosts/ui/hosts-page.tsx`, route `/operator/hosts` |
| Role | `roles.infra` in `config/shippin.json` |
| Approval | Empty configuration denies mutations; AgentHub-owned decision and ticket review integration remains pending |

Phase 1 HTTP does not register HA, SDN, Ceph, ACME, subscription, consoles, agent exec, or `/access/*`.
