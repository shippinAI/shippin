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
    <div data-testid="service-card" className="group relative overflow-hidden rounded-[var(--radius-card)] border border-line bg-surface p-5 shadow-[var(--shadow-e1)] transition-shadow hover:shadow-[var(--shadow-e2)]">
      <div className="absolute inset-x-0 top-0 h-px" style={{ background: `linear-gradient(90deg, transparent, ${k.hue}, transparent)` }} />
      <div className="flex items-start justify-between">
        <span className="flex size-10 items-center justify-center rounded-xl" style={{ background: `color-mix(in oklch, ${k.hue} 16%, transparent)` }}>
          <Icon className="size-5" style={{ color: k.hue }} strokeWidth={1.75} />
        </span>
        <Tooltip content={probe.latency_ms === undefined ? `Probe: ${probe.detail}` : `Probe: ${probe.detail} in ${probe.latency_ms} ms`}>
          <Focusable>
            <span tabIndex={0} className="outline-none">
              <Badge tone={probe.healthy ? "success" : probe.configured === false ? "neutral" : "danger"}>
                <StatusDot health={probe.healthy ? "up" : probe.configured === false ? "unknown" : "down"} />
                {probe.healthy ? "Operational" : probe.configured === false ? "Not set up" : "Down"}
              </Badge>
            </span>
          </Focusable>
        </Tooltip>
      </div>
      <div className="mt-4 text-[15px] font-semibold">{probe.name}</div>
      <div className="text-xs text-fg-subtle capitalize">{probe.kind}</div>
      <dl className="mt-4 flex gap-6 border-t border-line pt-3.5 text-xs">
        <Stat label="Latency" value={probe.latency_ms === undefined ? "—" : `${probe.latency_ms} ms`} />
        {/* Only a service tied to an instance has these to show. */}
        {instance && <Stat label="Memory" value={formatBytes(instance.memory_bytes)} />}
        {instance && <Stat label="Address" value={instance.ipv4 || "—"} mono />}
      </dl>
    </div>
  );
}

function Stat({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="min-w-0">
      <dt className="text-fg-subtle">{label}</dt>
      <dd className={`mt-0.5 truncate text-fg ${mono ? "font-mono text-[11px]" : ""}`}>{value}</dd>
    </div>
  );
}
