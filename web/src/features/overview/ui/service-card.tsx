import { Database, Globe, KeyRound, Fingerprint, Mail, Network, Server } from "lucide-react";
import { Badge } from "../../../lib/design/components/badge/badge";
import { StatusDot } from "../../../lib/design/components/status-dot/status-dot";
import { Tooltip } from "../../../lib/design/components/tooltip/tooltip";
import { Focusable } from "react-aria-components";
import type { Instance, Probe } from "../../../lib/api/types";
import { formatBytes } from "../../../lib/format";

const kinds: Record<string, { icon: typeof Server; hue: string }> = {
  nomen: { icon: Fingerprint, hue: "var(--color-hue-identity)" },
  vaultix: { icon: KeyRound, hue: "var(--color-hue-secrets)" },
  mail: { icon: Mail, hue: "var(--color-hue-edge)" },
  dns: { icon: Network, hue: "var(--color-hue-data)" },
  // Pre-rebrand probe kinds, so a config rollback still renders.
  infisical: { icon: KeyRound, hue: "var(--color-hue-secrets)" },
  caddy: { icon: Globe, hue: "var(--color-hue-edge)" },
  postgres: { icon: Database, hue: "var(--color-hue-data)" },
};

/** Live card: HTTP/TCP probe plus the backing Incus instance state. */
export function ServiceCard({ probe, instance }: { probe: Probe; instance?: Instance }) {
  const k = kinds[probe.kind] ?? { icon: Server, hue: "var(--color-primary)" };
  const Icon = k.icon;
  return (
    <div className="group relative overflow-hidden rounded-[var(--radius-card)] border border-line bg-surface p-5 shadow-[var(--shadow-e1)] transition-shadow hover:shadow-[var(--shadow-e2)]">
      <div className="absolute inset-x-0 top-0 h-px" style={{ background: `linear-gradient(90deg, transparent, ${k.hue}, transparent)` }} />
      <div className="flex items-start justify-between">
        <span className="flex size-10 items-center justify-center rounded-xl" style={{ background: `color-mix(in oklch, ${k.hue} 16%, transparent)` }}>
          <Icon className="size-5" style={{ color: k.hue }} strokeWidth={1.75} />
        </span>
        <Tooltip content={`Probe: ${probe.detail} in ${probe.latency_ms} ms`}>
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
      <dl className="mt-4 grid grid-cols-[0.8fr_1fr_1.25fr] gap-2 border-t border-line pt-3.5 text-xs">
        <Stat label="Latency" value={probe.detail === "not configured" ? "—" : `${probe.latency_ms} ms`} />
        <Stat label="Memory" value={formatBytes(instance?.memory_bytes ?? 0)} />
        <Stat label="Address" value={instance?.ipv4 ?? "—"} mono />
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
