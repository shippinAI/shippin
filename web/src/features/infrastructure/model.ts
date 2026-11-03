import type { Guest } from "../../lib/hooks/use-hosts";
import type { InfraField, InfraNode, InfraOverview, InfraProvider, InfraTotals, NodeDetails } from "../../lib/hooks/use-infra";

export const OVERVIEW_TAB = "overview";
export const FLEETS_TAB = "fleets";

/** A provider gets a tab once a host is attached to it, so tabs always show something real. */
export const providersWithHosts = (o: InfraOverview | undefined): InfraProvider[] => (o?.providers ?? []).filter((p) => p.nodes.length > 0);

/** The tab a URL asks for, or the overview when that provider has no host to show. */
export function selectedTab(o: InfraOverview | undefined, wanted: string | undefined): string {
  if (wanted === FLEETS_TAB) return FLEETS_TAB;
  return wanted && providersWithHosts(o).some((p) => p.kind === wanted) ? wanted : OVERVIEW_TAB;
}

export const tabHref = (id: string) => (id === OVERVIEW_TAB ? "/operator/hosts" : `/operator/hosts/${encodeURIComponent(id)}`);

export const slugFrom = (name: string) => name.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 40).replace(/-+$/, "");

export const slugOK = (s: string) => /^[a-z0-9][a-z0-9-]{0,39}$/.test(s);
export const siteOK = (s: string) => /^[a-z0-9-]{1,40}$/.test(s);

export const initialValues = (fields: InfraField[]): Record<string, string> => Object.fromEntries(fields.map((f) => [f.name, f.default ?? ""]));

export const fieldsComplete = (fields: InfraField[], values: Record<string, string>) => fields.every((f) => !f.required || !!values[f.name]?.trim());

/** Only fields that have a value are sent, so a blank optional field is not stored as a setting. */
export const filledValues = (fields: InfraField[], values: Record<string, string>): Record<string, string> =>
  Object.fromEntries(fields.flatMap((f) => (values[f.name]?.trim() ? [[f.name, values[f.name]!.trim()]] : [])));

/** Status words come straight from each hypervisor and differ in case between them. */
export const isRunning = (status: string) => status.toLowerCase() === "running";
export const isStopped = (status: string) => status.toLowerCase() === "stopped";

export type FleetMember = { node: InfraNode; providerLabel: string };

const apiHosts = (o: InfraOverview): FleetMember[] =>
  o.providers.flatMap((p) => p.nodes.filter((n) => n.source === "api").map((node) => ({ node, providerLabel: p.label })));

export const fleetMembers = (o: InfraOverview, fleetId: string): FleetMember[] => apiHosts(o).filter((m) => m.node.fleet_id === fleetId);

/** The local agent's view is not a host that was added, so it cannot join a fleet. */
export const hostsWithoutFleet = (o: InfraOverview): FleetMember[] => apiHosts(o).filter((m) => !m.node.fleet_id);

export function fleetTotals(members: FleetMember[]): InfraTotals {
  return members.reduce(
    (t, { node }) => ({ nodes: t.nodes + 1, online: t.online + (node.online ? 1 : 0), guests: t.guests + node.guests.total, running: t.running + node.guests.running }),
    { nodes: 0, online: 0, guests: 0, running: 0 },
  );
}

/** Running guests, the panel's own last so it is never first in line. */
export const stoppableGuests = (guests: Guest[]): Guest[] =>
  guests.filter((g) => isRunning(g.status)).sort((a, b) => Number(!!a.self) - Number(!!b.self));

export function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400), hours = Math.floor((seconds % 86400) / 3600), minutes = Math.floor((seconds % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
}

export function memoryPercent(d: NodeDetails | undefined): number | null {
  if (!d?.mem_total_bytes) return null;
  return Math.min(100, Math.round(((d.mem_used_bytes ?? 0) / d.mem_total_bytes) * 100));
}

/** A provider tab opens on a host that answers, so one dead host does not hide the others. */
export const firstAnswering = (nodes: InfraNode[]): InfraNode | undefined => nodes.find((n) => n.online) ?? nodes[0];
