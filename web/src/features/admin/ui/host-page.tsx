import { Activity, Cpu, Database, HardDrive, Layers, MemoryStick, PlugZap } from "lucide-react";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { useAdminHost } from "../../../lib/hooks/use-admin";
import { formatBytes } from "../../../lib/format";
import { PageHeader } from "../../../template/page-header";
import { QueryView } from "./query-view";
import { StatCard } from "./stat-card";
import { StatusBadge } from "./status-badge";

const pct = (a: number, b: number) => (b ? (100 * a) / b : 0);
const uptime = (s: number) => `${Math.floor(s / 86400)}d ${Math.floor((s % 86400) / 3600)}h`;

export function HostPage() {
  const q = useAdminHost();
  const s = q.data?.stats;
  const memUsed = s ? s.mem_total - s.mem_available : 0;
  const ports = s?.ssh_ports;
  const portTotal = ports ? ports.max - ports.min + 1 : 0;
  const inst = s?.instances ?? {};
  return (
    <>
      <PageHeader eyebrow="Provider" title={`Host · ${q.data?.label ?? "…"}`} description={s ? `Up ${uptime(s.uptime_s)} · ${s.cpus} CPUs · refreshes every 10s` : "Live host metrics from shippin-hostd."} />
      <QueryView isPending={q.isPending} error={q.error} onRetry={() => void q.refetch()}>
        {() => s && (
          <>
            <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
              <StatCard label="CPU" value={`${s.cpu_pct.toFixed(1)}%`} pct={s.cpu_pct} icon={Cpu} hue="var(--color-primary)" hint={`${s.cpus} logical CPUs`} />
              <StatCard label="Load (1/5/15m)" value={s.load.map((l) => l.toFixed(2)).join(" · ")} pct={pct(s.load[0], s.cpus)} icon={Activity} hue="var(--color-hue-ai)" hint="Bar = 1m load / CPUs" />
              <StatCard label="Memory" value={`${pct(memUsed, s.mem_total).toFixed(0)}%`} pct={pct(memUsed, s.mem_total)} icon={MemoryStick} hue="var(--color-hue-data)" hint={`${formatBytes(memUsed)} of ${formatBytes(s.mem_total)}`} />
              <StatCard label="SSH port pool" value={`${ports?.used ?? 0} / ${portTotal}`} pct={pct(ports?.used ?? 0, portTotal)} icon={PlugZap} hue="var(--color-hue-edge)" hint={ports ? `${ports.min}–${ports.max} on the shared endpoint` : ""} />
            </div>
            <div className="mt-5 grid gap-5 lg:grid-cols-[3fr_2fr]">
              <Card className="overflow-hidden">
                <CardHeader title="ZFS pools" description="zpool list" />
                <div className="divide-y divide-line">
                  {s.zfs_pools.length === 0 && <div className="px-5 py-4 text-sm text-fg-subtle">No pools reported.</div>}
                  {s.zfs_pools.map((p) => (
                    <div key={p.name} className="px-5 py-4">
                      <div className="flex items-center gap-3 text-sm">
                        <HardDrive className="size-4 text-fg-subtle" />
                        <span className="font-mono font-medium text-fg">{p.name}</span>
                        <StatusBadge status={p.health === "ONLINE" ? "active" : p.health} />
                        <span className="ml-auto text-fg-muted tabular-nums">{formatBytes(p.alloc)} / {formatBytes(p.size)}</span>
                      </div>
                      <div className="mt-2.5 h-2 overflow-hidden rounded-full bg-raised" role="meter" aria-label={`${p.name} usage`} aria-valuenow={Math.round(pct(p.alloc, p.size))}>
                        <div className="h-full rounded-full bg-[var(--color-hue-data)]" style={{ width: `${pct(p.alloc, p.size)}%` }} />
                      </div>
                      <div className="mt-1.5 text-xs text-fg-subtle">{pct(p.alloc, p.size).toFixed(1)}% used · {formatBytes(p.free)} free</div>
                    </div>
                  ))}
                </div>
              </Card>
              <Card className="overflow-hidden">
                <CardHeader title="Incus" description="All projects" />
                <div className="grid grid-cols-2 gap-3 p-4">
                  <StatCard label="Projects" value={s.projects} icon={Layers} hue="var(--color-primary)" />
                  <StatCard label="Instances" value={Object.values(inst).reduce((a, b) => a + b, 0)} icon={Database} hue="var(--color-hue-workspaces)" />
                  <StatCard label="Running" value={inst.running ?? 0} icon={Activity} hue="var(--color-success)" />
                  <StatCard label="Stopped" value={inst.stopped ?? 0} icon={Activity} hue="var(--color-fg-muted)" />
                </div>
              </Card>
            </div>
          </>
        )}
      </QueryView>
    </>
  );
}
