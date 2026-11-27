# Metrics

How the panel records every host's and guest's CPU, memory, disk and network, and how it reads them
back (OBS-01 in [What Shippin is](objective.md), roadmap stage 3). The code is
`internal/infrastructure/metric`; the drivers' readings are `hv.HostMeter` and `hv.GuestMeter` in
`internal/hv`.

## Recording

- **Settings > Metrics** (section `metrics`): record metrics on or off (default on), sample every
  10 to 3600 seconds (default 30), keep samples 1 to 365 days (default 30). A save takes effect without
  a restart; a new keep days changes the ClickHouse table's TTL, and ClickHouse removes older rows in
  the background.
- Each tick reads every attached host and its guests, at most 8 hosts at once and each within its own
  timeout (10 seconds, or the interval if shorter), so one slow host never holds up the rest. The tick
  writes one batch insert to `shippin_metric_samples`.
- One panel process samples at a time. It holds a PostgreSQL session advisory lock on one connection;
  the others stand by and take over on their next tick when that process stops or loses its database.
  A lock rather than the job queue, because the rates below need the previous reading in the memory of
  the one process that took it, and the sampler must run with the job queue turned off.
- With ClickHouse not connected, or recording off, the sampler writes nothing, gives up the lock and
  says why in its status. The range read below returns that status as `sampler`.

## Metric names

A host's samples have `guest_id` empty; a guest's carry the hypervisor's id for it (a Proxmox VMID, an
Incus instance name).

| Name | Unit | Meaning |
| --- | --- | --- |
| `cpu` | fraction, 0 to 1 | Busy share of all the machine's CPUs: the host's, or the CPUs the guest may use. |
| `load1` | load | One-minute load average. Hosts only, where the provider reports it. |
| `memory_used_bytes` | bytes | Memory in use. |
| `memory_total_bytes` | bytes | Memory the machine has, or the guest's limit. |
| `disk_used_bytes` | bytes | A host's root filesystem; a guest's disks. |
| `disk_total_bytes` | bytes | The same, total size. |
| `net_in_bytes_per_second` | bytes per second | Traffic into the machine, over all interfaces but loopback. |
| `net_out_bytes_per_second` | bytes per second | Traffic out of it. |

Missing is never zero:
- A value the provider did not report is not written.
- A guest that is not running is not sampled.
- A cumulative counter (CPU time, network bytes) becomes a rate from two readings held in memory. The
  first reading after a start, a counter that went backwards (a restart), and a previous reading older
  than ten minutes (or three intervals) give no rate for that step.
- While recording is off or ClickHouse is away, nothing is written. Each of these is a gap.

## What each driver provides

| | Proxmox VE | Incus |
| --- | --- | --- |
| Host `cpu` | `/nodes/{node}/status` `cpu` | The host monitor: the busy share of all CPUs over one second, from `/proc/stat`. Not read without the monitor |
| Host `load1` | `/nodes/{node}/status` `loadavg` | The host monitor: `/proc/loadavg`. Not read without the monitor |
| Host memory | `/nodes/{node}/status` `memory` | Total from `/1.0/resources`. In use from the host monitor: `MemTotal` less `MemAvailable` in `/proc/meminfo`. Not read without the monitor |
| Host disk | `/nodes/{node}/status` `rootfs` | Not read: storage pools are not the host's disk, and several can share one filesystem |
| Host network | The node's newest one-minute average in `/nodes/{node}/rrddata`, if under five minutes old | Not read |
| Guest `cpu` | `/cluster/resources` `cpu`, already a fraction | Rate of `state.cpu.usage` (nanoseconds) over `limits.cpu`, or the host's CPUs for a container without one, or one CPU for a VM without one |
| Guest memory | `mem`, `maxmem` | `state.memory.usage`, `total` |
| Guest disk | Containers: `disk`, `maxdisk`. VMs: `maxdisk` only, because Proxmox reports a VM's used disk as 0 | `state.disk` usage and total, summed over the guest's disks |
| Guest network | Rate of `netin`, `netout` | Rate of `state.network.*.counters`, from inside the guest |
| Calls per host per tick | `/nodes/{node}/status`, `/nodes/{node}/rrddata`, `/cluster/resources` | `/1.0`, `/1.0/resources`, one `/1.0/instances?recursion=2`, and with the monitor one recorded command and its output |

### The Incus host monitor

Incus answers `/1.0/resources` from what it read when its daemon started. On a real host the memory in
use and the load it gave there did not change in over two hours, so they are not sampled. The panel
reads the host through a monitor instead, which someone turns on per server under **Host readings**
(`shippin.infrastructure.server.host_monitor.update`). Nothing is installed on the host:

- Turning it on makes one container named `shippin-host-monitor` through the Incus API, from an image
  the person names. It has no network, one CPU, 64 MiB of memory and the host's `/proc` read-only.
- Each tick the panel runs one recorded command in it, reads the output and removes the recorded files.
- Turning it off removes the container. Without a monitor the host's CPU, load and memory in use are
  missing, never estimated.

For a Proxmox cluster the host is the attached node, and the guests are every guest in the cluster,
each with the node it runs on. Incus reports 0 or less for a VM's CPU time or memory when it cannot read
them, such as without its agent; those are not written. Hosts with no hypervisor API (bare) and the
local host agent are not sampled.

Not yet proved against a real Proxmox or Incus: the drivers' readings are tested against recorded API
shapes. The sampler and the range read are tested against a real ClickHouse and PostgreSQL.

## Reading a range

`shippin.infrastructure.metric.list`, `GET /v1/operator/infrastructure/metrics`, staff with
`infra.read`. Query parameters, all text:

| Parameter | |
| --- | --- |
| `server_id` | The host's ID. |
| `guest_id` | One guest's id; empty for the host itself. |
| `metrics` | Names from the table above, separated by commas, 1 to 8. |
| `from`, `to` | RFC 3339 times. `from` is rounded down to a whole step; `to` is not included. |
| `step` | Bucket length in whole seconds, at least 10. |

At most 92 days and 1500 steps per range; beyond either it is refused with a message that says which,
and for too many steps the smallest step that fits. An unknown metric is refused with the list of
names; an unknown host is a 404; with ClickHouse not connected it is a 503 saying so.

The answer has `times`, the start of every bucket, and one entry in `series` per metric asked for,
with its `unit` and `buckets` in the same order as `times`. A bucket is `{avg, max, samples}`, or
`null` when there were no samples in that step. `sampler` says what the answering panel process's
sampler is doing: `state` (sampling, standby, idle, off or starting), `reason`, `interval_seconds` and
`last_sampled_at`.

## Retention

Samples are kept for the section's keep days (30 by default) through the table's TTL, in monthly
partitions. ClickHouse history is not backed up yet.
