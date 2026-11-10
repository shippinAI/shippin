import { Database, Globe, KeyRound, Fingerprint, Mail, Network, Server } from "lucide-react";
import { Badge } from "../../../lib/design/components/badge/badge";
import { StatusDot } from "../../../lib/design/components/status-dot/status-dot";
import { Tooltip } from "../../../lib/design/components/tooltip/tooltip";
import { Focusable } from "react-aria-components";
import type { Instance, Probe } from "../../../lib/api/types";
import { formatBytes } from "../../../lib/format";

// The kind is whatever the configuration calls it. These categories only choose an icon, and any other
// kind gets the plain one.
const kinds: Record<string, { icon: typeof Server; hue: string }> = {
  identity: { icon: Fingerprint, hue: "var(--color-hue-identity)" },
  secrets: { icon: KeyRound, hue: "var(--color-hue-secrets)" },
  mail: { icon: Mail, hue: "var(--color-hue-edge)" },
  dns: { icon: Network, hue: "var(--color-hue-data)" },
  proxy: { icon: Globe, hue: "var(--color-hue-edge)" },
  database: { icon: Database, hue: "var(--color-hue-data)" },
};

/** Live card: HTTP/TCP probe plus the backing Incus instance state. */
export function ServiceCard({ probe, instance }: { probe: Probe; instance?: Instance }) {
  const k = kinds[probe.kind.toLowerCase()] ?? { icon: Server, hue: "var(--color-primary)" };
  const Icon = k.icon;
  return (
    <div data-testid="service-card" className="group relative overflow-hidden rounded-xl border border-line bg-surface p-5 shadow-e1 transition-shadow hover:shadow-e2">
      <div className="absolute inset-x-0 top-0 h-px" style={{ background: `linear-gradient(90deg, transparent, ${k.hue}, transparent)` }} />
      <div className="flex items-start justify-between">
        <span className="flex size-10 items-center justify-center rounded-xl" style={{ background: `color-mix(in oklch, ${k.hue} 16%, transparent)` }}>
          <Icon className="size-5" style={{ color: k.hue }} strokeWidth={1.75} />
        </span>
        <Tooltip content={probe.latency_ms === undefined ? `Probe: ${probe.detail}` : `Probe: ${probe.detail} in ${probe.latency_ms} ms`}>
          <Focusable>
            <span tabIndex={0} className="outline-none">
              <Badge tone={probe.state === "operational" || (!probe.state && probe.healthy) ? "success" : probe.state === "down" || (!probe.state && probe.configured !== false) ? "danger" : probe.state === "stale" ? "warning" : "neutral"}>
                <StatusDot health={probe.state === "operational" || (!probe.state && probe.healthy) ? "up" : probe.state === "down" || (!probe.state && probe.configured !== false) ? "down" : "unknown"} />
                {probe.state === "operational" || (!probe.state && probe.healthy) ? "Operational" : probe.state === "stale" ? "Stale" : probe.state === "down" || (!probe.state && probe.configured !== false) ? "Down" : "Unknown"}
              </Badge>
            </span>
          </Focusable>
        </Tooltip>
      </div>
      <div className="mt-4 text-lg font-semibold">{probe.alias || probe.name}</div>
      <div className="text-xs text-fg-subtle capitalize">{probe.kind}</div>
      <dl className="mt-4 flex gap-6 border-t border-line pt-3.5 text-xs">
        <Stat label="Latency" value={probe.latency_ms === undefined ? "—" : `${probe.latency_ms} ms`} />
        {/* Only a service tied to an instance has these to show. */}
        {instance && <Stat label="Memory" value={formatBytes(instance.memory_bytes)} />}
        {instance && <Stat label="Address" value={instance.ipv4 || "—"} mono />}
      </dl>
      {probe.resource && (
        <div className="mt-3 border-t border-line pt-3 text-xs">
          <div className="mb-2 flex flex-wrap items-center justify-between gap-2 text-fg-subtle">
            <span>Provider resource · {probe.resource.server || probe.resource.server_id}</span>
            <span>{probe.resource.status ?? (probe.resource.observed ? "Observed" : "Unknown")}</span>
          </div>
          {probe.resource.observed && probe.resource.metrics ? (
            <dl className="grid grid-cols-3 gap-3">
              <Stat label="CPU" value={`${(probe.resource.metrics.cpu_fraction * 100).toFixed(1)}%`} />
              <Stat label="Memory" value={`${formatBytes(probe.resource.metrics.memory_used_bytes)} / ${formatBytes(probe.resource.metrics.memory_total_bytes)}`} />
              <Stat label="Disk" value={`${formatBytes(probe.resource.metrics.disk_used_bytes)} / ${formatBytes(probe.resource.metrics.disk_total_bytes)}`} />
            </dl>
          ) : (
            <p className="text-fg-subtle">{probe.resource.observed ? probe.resource.detail : `Resource state unavailable: ${probe.resource.detail}`}</p>
          )}
          {probe.resource.observed && (
            <div className="mt-2 text-fg-subtle">
              <span>Provider addresses: </span>
              {probe.resource.addresses?.length ? probe.resource.addresses.map((address) => `${address.interface}: ${address.address}`).join(", ") : "unknown"}
              <p className="mt-1">{probe.resource.network_detail || "Provider observation does not verify probe reachability."}</p>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function Stat({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="min-w-0">
      <dt className="text-fg-subtle">{label}</dt>
      <dd className={`mt-0.5 truncate text-fg ${mono ? "font-mono text-2xs" : ""}`}>{value}</dd>
    </div>
  );
}
