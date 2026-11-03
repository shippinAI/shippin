# Per-org storage

Status: 2026-10-22. Every byte a tenant owns lives on that tenant's dataset
`tank/tenants/<tenant>` (Incus pool `p-<tenant>`). A tenant project cannot use another pool:
provision sets `limits.disk.pool.<hidden>=0` for every pool in `hidden_pools` (the shipped file
lists `platform`).

Nothing here is deployed. Gold's live hostd is a different config.

## Where each byte goes

| Byte | Where | On the org dataset? |
|---|---|---|
| Tenant VM root disk | zvol on `p-<tenant>`, device `root` of `tvm-<tenant>` | yes |
| Tenant VM data volume | block volume `tvd-<tenant>` on `p-<tenant>` | yes |
| Inner Incus pool | btrfs on that data disk. The pool **name** inside the guest is `default`. `shippin-tvm-init` sets `user.shippin.tenant` on it. | yes (the zvol) |
| Workspace containers | host pool `p-<tenant>` when the plan has no VM (`free`); inner btrfs when it does | yes |
| `~/.ai` (`ai-<account>`) | `place.pool()`: `p-<tenant>` or the inner pool | yes |
| Cowork's ~25 GiB disk | not created. When it is, it is a volume on the data disk, never the VM root. `cowork: true` plans have a data volume of at least 25 GiB and at least 8 GiB of VM RAM. | yes, by the check |
| Workspace `pub-*` snapshots | the instance's own disk (org pool or inner btrfs) | yes |
| ZFS dataset snapshots | `tank/tenants/<tenant>` when someone creates them. No schedule is implemented. | yes |
| Git publish / site-scan scratch | `tank/tenants/<tenant>/scratch`, removed after the scan. Refuses a mountpoint whose last element is not the tenant id. | yes |
| Image import scratch inside the VM | `/var/lib/incus/storage-pools/default/.shippin-img/`, deleted after import | yes (data volume) |

The dataset is created with `shippin:tenant=<tenant>` and `quota=<plan>`. The project carries
`user.shippin.tenant` and `user.shippin.plan`. The data volume carries `user.shippin.tenant`.

## What is shared, on purpose

- **Platform image cache** `<state_dir>/images/<fingerprint>`. These are template images, not a
  tenant's files. The copy that lands in a VM is imported onto that org's data volume.
- **Starter Postgres** (`starter_db`), when enabled. One platform cluster, one database per
  tenant. That is the free/starter Vaultix exception (shared Postgres, per-tenant section).
  Pro and above are marked dedicated in the catalog and hostd does not provision that dedicated
  database yet.
- **gitleaks**, the edge, and the console. Tools and platform services, not tenant disks.
- **Connect spool** on `/run/shippin`. Short-lived tmpfs for a login, not a durable copy of
  `~/.ai`.

## What was leaving the org dataset

- Git publish and the site scanner wrote the devspace bundle and working tree to the host's
  default temp directory (`os.MkdirTemp("", …)`), which is not `tank/tenants/<tenant>`. They now
  use the dataset mount and delete it when the call returns.
- Template import staged files on the VM root (`/root/shippin-img`), the 20 GiB disk. Staging is
  now the inner pool mount, which is the data volume.
- A plan change updated the Incus project limit and did not raise the ZFS quota or the VM disks
  if the dataset already existed. Re-provision and `POST /tenants/{id}/plan` now grow.

## Growing

Shrinking is refused. Sizes are absolute GiB. A zero field is left alone.

`POST /tenants/{tenant}/storage` body `{"quota_gib","root_gib","data_gib"}`:

1. `zfs set quota=<n>G tank/tenants/<tenant>`
2. `limits.disk.pool.p-<tenant>=<n>GiB` on the tenant project
3. VM root device `size=`, then the data volume `tvd-<tenant>` `size=`
4. Inside the guest, `shippin-tvm-init grow <tenant>` rescans the data disk, grows the root
   filesystem (growpart + resize2fs or btrfs), and `btrfs filesystem resize max` on the inner pool

`POST /tenants/{tenant}/plan` body `{"plan":"pro"}` records the plan, creates the VM when the
new plan has one and the tenant does not, and grows disks up to the plan. It will not delete a
VM to apply `free`. CPU and memory on an existing VM are written with `incus config set` and
are not restarted; the response sets `restart_required` when those values changed.

Same calls from the host:

```sh
shippin-hostd storage show --tenant <uuid>
shippin-hostd storage add --tenant <uuid> --quota-gib N --root-gib N --data-gib N
```

The CLI talks to the running daemon on its unix socket. It does not grow disks itself.

`GET /tenants/{tenant}/storage` and `GET /tenants/{tenant}/vm` set `low_space` when either:

- free space on the org dataset is under 10% of the quota, or
- the plan has `cowork: true` and the data volume has under 25 GiB free

Both thresholds are proposals. If the quota cannot be read, `low_space` is true and
`low_space_reason` says so.

## `~/.ai` and `security.shifted`

`deploy/hostd.json` keeps `ai.volume_shifted` **false**. Isolated idmaps mean the volume's
owner is correct in one container and `nobody` in the next. `security.shifted=true` is the
Incus fix, and creating the volume with that property fails the whole AI home if this ZFS
pool rejects it. That is not safe to flip from here.

On gold, against a throwaway tenant (not founder):

```sh
zfs get -H encryption,mountpoint,quota tank/tenants/<tenant>
incus storage volume create p-<tenant> shift-probe --project t-<tenant> size=1GiB security.shifted=true
incus storage volume get p-<tenant> shift-probe security.shifted --project t-<tenant>
```

If create returns an error, quote it and leave `volume_shifted` false. The usual miss is
idmapped mounts on this ZFS dataset (the kernel and the ZFS module have to allow them; Incus
6.0.6 will not invent that). If create succeeds, attach the volume to two containers that both
have `security.idmap.isolated=true` and confirm uid 1000 inside each sees the files as itself.
Then:

```sh
incus storage volume delete p-<tenant> shift-probe --project t-<tenant>
```

Only after that probe succeeds should `ai.volume_shifted` become true on gold. `ownAIHome`
chowns the mount point, not the files inside it, so it is not a substitute.
