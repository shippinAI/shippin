import type { Guest } from "../../lib/hooks/use-hosts";
import type { HvDetection } from "../../lib/api/actions.gen";
import type { InfraField, InfraNode, InfraOverview, InfraProvider, InfraTotals, NodeDetails } from "../../lib/hooks/use-infra";
import type { MetricBucket, MetricName, MetricRange, MetricSeries, MetricUnit } from "../../lib/hooks/use-metrics-history";
import type { ChartPoint, ChartUnit } from "../../lib/design/components/chart/chart-model";

export const OVERVIEW_TAB = "overview";
export const FLEETS_TAB = "fleets";
export const NETWORKS_TAB = "networks";

/** A provider gets a tab once a host is attached to it, so tabs always show something real. */
export const providersWithHosts = (o: InfraOverview | undefined): InfraProvider[] => (o?.providers ?? []).filter((p) => p.nodes.length > 0);

/** The tab a URL asks for, or the overview when that provider has no host to show. */
export function selectedTab(o: InfraOverview | undefined, wanted: string | undefined): string {
  if (wanted === FLEETS_TAB) return FLEETS_TAB;
  if (wanted === NETWORKS_TAB) return NETWORKS_TAB;
  return wanted && providersWithHosts(o).some((p) => p.kind === wanted) ? wanted : OVERVIEW_TAB;
}

const infraBase = () => (typeof location !== "undefined" && location.pathname.startsWith("/admin/") ? "/admin/infrastructure" : "/operator/hosts");

/** Where a tab of this page lives. It is reachable from the console and from the workspace shell. */
export const tabHref = (id: string, base = infraBase()) => (id === OVERVIEW_TAB ? base : `${base}/${encodeURIComponent(id)}`);

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
/** A guest name that either hypervisor accepts. */
export const guestNameOK = (name: string) => /^[a-zA-Z][a-zA-Z0-9-]{0,62}$/.test(name);

/** The images that suit this kind of guest. An image with no kind suits either. */
export const imagesFor = <T extends { kind?: string }>(images: T[], kind: string) => images.filter((i) => !i.kind || i.kind === kind);

/** Network types that can attach a guest to a software bridge. */
export const guestNetworks = <T extends { type: string }>(networks: T[]) => networks.filter((n) => ["bridge", "ovsbridge"].includes(n.type.toLowerCase()));

/** A change must say why. The server applies the same limits. */
export const noteOK = (note: string) => {
  const n = [...note.trim()].length;
  return n >= 5 && n <= 300;
};

/** Whether a verb can do anything from this status. A status this panel does not know is left to the hypervisor. */
export const verbApplies = (verb: string, status: string) => {
  if (isRunning(status)) return verb !== "start";
  if (isStopped(status)) return verb === "start";
  return true;
};

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

/** What confirming a detection puts in the add form: the provider, its fields with the base URL filled in
 * where the provider takes one, and the certificate to trust when the panel does not trust it already.
 * Nothing when the finding is not a provider this panel can add. */
export type DetectionPrefill = { kind: string; values: Record<string, string>; trusted: string };

export function prefillFromDetection(d: HvDetection | undefined, providers: InfraProvider[]): DetectionPrefill | undefined {
  if (!d || d.outcome !== "recognised" || !d.kind) return undefined;
  const provider = providers.find((p) => p.attachable && p.kind === d.kind);
  if (!provider) return undefined;
  const values = initialValues(provider.fields);
  if (d.base_url && provider.fields.some((f) => f.name === "base_url")) values.base_url = d.base_url;
  return { kind: provider.kind, values, trusted: d.certificate && !d.certificate_verified ? d.certificate.pem : "" };
}

/** Whether a certificate a test met is the one detection showed. Fingerprints are compared as hex,
 * ignoring separators and case. */
export const sameCertificate = (a: string | undefined, b: string | undefined) => {
  const norm = (s: string | undefined) => (s ?? "").replace(/[^0-9a-f]/gi, "").toLowerCase();
  return norm(a) !== "" && norm(a) === norm(b);
};

/** A provider tab opens on a host that answers, so one dead host does not hide the others. */
export const firstAnswering = (nodes: InfraNode[]): InfraNode | undefined => nodes.find((n) => n.online) ?? nodes[0];

export type FleetRow = {
  /** Unique across providers: kind and host id. */
  id: string;
  node: InfraNode;
  kind: string;
  providerLabel: string;
  fleetLabel: string;
  health: InfraNode["health_state"];
  cpus: number | null;
  memUsed: number | null;
  memTotal: number | null;
  memFraction: number | null;
  /** Raw capacity of the disks the host reported; their use is not in the overview. */
  diskBytes: number | null;
  diskCount: number;
  guestsRunning: number;
  guestsTotal: number;
};

const healthOf = (n: InfraNode): InfraNode["health_state"] => n.health_state ?? (n.online ? "operational" : "down");

/** Every host of every provider in one list, with the resources the overview reports. Unknown stays null. */
export function fleetRows(o: InfraOverview): FleetRow[] {
  return o.providers.flatMap((p) => p.nodes.map((node): FleetRow => {
    const d = node.details;
    const disks = d?.disks ?? [];
    const memTotal = d?.mem_total_bytes ? d.mem_total_bytes : null;
    const memUsed = memTotal !== null && d?.mem_used_bytes !== undefined ? d.mem_used_bytes : null;
    return {
      id: `${p.kind}:${node.id}`, node, kind: p.kind, providerLabel: p.label,
      fleetLabel: o.fleets.find((f) => f.id === node.fleet_id)?.label ?? "",
      health: healthOf(node),
      cpus: d?.cpus ? d.cpus : null,
      memUsed, memTotal, memFraction: memUsed !== null && memTotal ? Math.min(1, memUsed / memTotal) : null,
      diskBytes: disks.length ? disks.reduce((s, x) => s + (x.size_bytes || 0), 0) : null,
      diskCount: disks.length,
      guestsRunning: node.guests.running, guestsTotal: node.guests.total,
    };
  }));
}

export type FleetSortKey = "name" | "provider" | "site" | "state" | "cpu" | "memory" | "disk" | "guests" | "fleet";
export type SortDirection = "ascending" | "descending";

/** Ascending state puts trouble first: down, then unknown, then operational. */
const stateRank = { down: 0, unknown: 1, operational: 2 } as const;

const sortValue = (r: FleetRow, key: FleetSortKey): string | number | null => {
  switch (key) {
    case "name": return r.node.label.toLowerCase();
    case "provider": return r.providerLabel.toLowerCase();
    case "site": return r.node.site?.toLowerCase() || null;
    case "fleet": return r.fleetLabel.toLowerCase() || null;
    case "state": return stateRank[r.health] ?? 1;
    case "cpu": return r.cpus;
    case "memory": return r.memFraction;
    case "disk": return r.diskBytes;
    case "guests": return r.guestsTotal ? r.guestsRunning + r.guestsTotal / 1e6 : 0;
  }
};

/** Sorts the fleet by one column. A host that did not report that number goes last either way; ties go by name. */
export function sortFleet(rows: FleetRow[], key: FleetSortKey, direction: SortDirection): FleetRow[] {
  const sign = direction === "ascending" ? 1 : -1;
  const byName = (a: FleetRow, b: FleetRow) => a.node.label.localeCompare(b.node.label) || a.id.localeCompare(b.id);
  return [...rows].sort((a, b) => {
    const va = sortValue(a, key), vb = sortValue(b, key);
    if (va === null || vb === null) return va === vb ? byName(a, b) : va === null ? 1 : -1;
    const c = typeof va === "number" && typeof vb === "number" ? va - vb : String(va).localeCompare(String(vb));
    return c ? c * sign : byName(a, b);
  });
}

/** Where a host's detail lives: its provider's tab, with the host chosen. */
export const hostHref = (kind: string, hostId: string, base?: string) => `${tabHref(kind, base)}?host=${encodeURIComponent(hostId)}`;

export type HistoryChartId = "cpu" | "load" | "memory" | "disk" | "network";
export type HistoryChart = {
  id: HistoryChartId;
  unit: ChartUnit;
  /** The axis reaches at least this, such as total memory, so use reads against the whole. */
  ceiling?: number;
  series: { id: string; metric: MetricName; part: "avg" | "max"; points: ChartPoint[] }[];
};

const chartUnit: Record<MetricUnit, ChartUnit> = { fraction: "fraction", load: "count", bytes: "bytes", bytes_per_second: "bytes_per_second" };

const chartPlan: { id: HistoryChartId; parts: [MetricName, "avg" | "max"][]; total?: MetricName }[] = [
  { id: "cpu", parts: [["cpu", "avg"], ["cpu", "max"]] },
  { id: "load", parts: [["load1", "avg"]] },
  { id: "memory", parts: [["memory_used_bytes", "avg"]], total: "memory_total_bytes" },
  { id: "disk", parts: [["disk_used_bytes", "avg"]], total: "disk_total_bytes" },
  { id: "network", parts: [["net_in_bytes_per_second", "avg"], ["net_out_bytes_per_second", "avg"]] },
];

/** A step with no sample is null; a bucket that claims no samples is missing too. */
const bucketValue = (b: MetricBucket | null | undefined, part: "avg" | "max"): number | null =>
  !b || b.samples === 0 || !Number.isFinite(b[part]) ? null : b[part];

/**
 * The charts a range response fills. Buckets line up with `times`; an unreadable time is dropped.
 * A chart whose series have no sample at all is left out, so a range with nothing shows the empty state.
 */
export function historyCharts(h: MetricRange | undefined): HistoryChart[] {
  if (!h) return [];
  const times = h.times.map((s) => Date.parse(s));
  const byMetric = new Map(h.series.map((s) => [s.metric, s]));
  const pointsOf = (s: MetricSeries, part: "avg" | "max"): ChartPoint[] =>
    times.flatMap((at, i) => (Number.isFinite(at) ? [{ at, value: bucketValue(s.buckets[i], part) }] : []));
  const sampled = (s: MetricSeries | undefined) => !!s && s.buckets.some((b) => bucketValue(b, "avg") !== null);
  const charts: HistoryChart[] = [];
  for (const c of chartPlan) {
    const parts = c.parts.filter(([m]) => sampled(byMetric.get(m)));
    if (!parts.length) continue;
    const totals = c.total ? byMetric.get(c.total)?.buckets.map((b) => bucketValue(b, "max")).filter((v): v is number => v !== null) ?? [] : [];
    charts.push({
      id: c.id, unit: chartUnit[byMetric.get(parts[0]![0])!.unit] ?? "count",
      ceiling: totals.length ? Math.max(...totals) : undefined,
      series: parts.map(([metric, part]) => ({ id: `${metric}.${part}`, metric, part, points: pointsOf(byMetric.get(metric)!, part) })),
    });
  }
  return charts;
}
