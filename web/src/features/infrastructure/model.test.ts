import assert from "node:assert/strict";
import type { InfraField, InfraOverview, InfraProvider } from "../../lib/hooks/use-infra";
import type { HvDetection } from "../../lib/api/actions.gen";
import {
  guestNetworks,
  noteOK,
  verbApplies,
  firstAnswering,
  FLEETS_TAB,
  NETWORKS_TAB,
  fieldsComplete,
  filledValues,
  fleetMembers,
  fleetTotals,
  formatUptime,
  guestNameOK,
  hostsWithoutFleet,
  imagesFor,
  initialValues,
  isRunning,
  memoryPercent,
  OVERVIEW_TAB,
  prefillFromDetection,
  providersWithHosts,
  sameCertificate,
  selectedTab,
  slugFrom,
  slugOK,
  stoppableGuests,
  tabHref,
} from "./model";
import { fleetRows, historyCharts, hostHref, sortFleet, type FleetSortKey } from "./model";
import type { InfraNode } from "../../lib/hooks/use-infra";
import type { MetricRange } from "../../lib/hooks/use-metrics-history";

const totals = { nodes: 0, online: 0, guests: 0, running: 0 };
const provider = (kind: string, nodes = 0): InfraProvider => ({
  kind,
  label: kind,
  attachable: true,
  capabilities: [],
  verbs: [],
  fields: [],
  totals,
  nodes: Array.from({ length: nodes }, (_, i) => ({
    id: `${kind}${i}`,
    label: "x",
    kind,
    source: "api" as const,
    online: true,
    guests: { total: 0, running: 0 },
  })),
});
const overview = (...providers: InfraProvider[]): InfraOverview => ({ providers, totals, fleets: [], unattached: [], checked_at: "" });

const none = overview(provider("first"), provider("second"));
assert.deepEqual(providersWithHosts(none), [], "no host, no tab");
assert.deepEqual(providersWithHosts(undefined), []);

const one = overview(provider("first", 1), provider("second"));
assert.deepEqual(
  providersWithHosts(one).map((p) => p.kind),
  ["first"],
  "adding a host adds that provider's tab",
);
const both = overview(provider("first", 1), provider("second", 2));
assert.deepEqual(
  providersWithHosts(both).map((p) => p.kind),
  ["first", "second"],
);

assert.equal(selectedTab(one, "first"), "first");
assert.equal(selectedTab(one, "second"), OVERVIEW_TAB, "a provider with no host falls back to the overview");
assert.equal(selectedTab(one, "not-a-provider"), OVERVIEW_TAB);
assert.equal(selectedTab(one, undefined), OVERVIEW_TAB);
assert.equal(selectedTab(undefined, "first"), OVERVIEW_TAB, "while loading");

assert.equal(tabHref(OVERVIEW_TAB), "/operator/hosts");
assert.equal(tabHref("first"), "/operator/hosts/first");

assert.equal(slugFrom("  Lab Rack #1 "), "lab-rack-1");
assert.equal(slugFrom("---"), "");
assert.equal(slugFrom("x".repeat(60)).length, 40);
assert.ok(slugOK(slugFrom("Lab Rack")));
assert.ok(!slugOK(""));
assert.ok(!slugOK("-a"));
assert.ok(!slugOK("A"));

const fields: InfraField[] = [
  { name: "a", required: true, label: "A", default: "preset" },
  { name: "b", required: true, label: "B" },
  { name: "c", required: false, label: "C" },
];
assert.deepEqual(initialValues(fields), { a: "preset", b: "", c: "" }, "defaults come from the field, not from code");
assert.equal(fieldsComplete(fields, { a: "x", b: "" }), false);
assert.equal(fieldsComplete(fields, { a: "x", b: "   " }), false, "spaces are not a value");
assert.equal(fieldsComplete(fields, { a: "x", b: "y" }), true, "an optional field may stay empty");
assert.deepEqual(filledValues(fields, { a: " x ", b: "", c: "  " }), { a: "x" });

assert.ok(isRunning("Running"));
assert.ok(isRunning("running"));
assert.ok(!isRunning("stopped"));

assert.equal(selectedTab(one, FLEETS_TAB), FLEETS_TAB, "the fleets tab exists before any host does");
assert.equal(selectedTab(undefined, FLEETS_TAB), FLEETS_TAB);
assert.equal(selectedTab(undefined, NETWORKS_TAB), NETWORKS_TAB, "network integrations are configurable before any host exists");

const node = (id: string, fleet_id?: string, online = true, source: "api" | "agent" = "api") => ({
  id,
  label: id,
  kind: "k",
  source,
  online,
  fleet_id,
  guests: { total: 3, running: 2 },
});
const mixed = overview(
  { ...provider("first"), label: "First", nodes: [node("a", "f1"), node("b"), node("local", undefined, true, "agent")] },
  { ...provider("second"), label: "Second", nodes: [node("c", "f1", false)] },
);
assert.deepEqual(
  fleetMembers(mixed, "f1").map((m) => [m.node.id, m.providerLabel]),
  [
    ["a", "First"],
    ["c", "Second"],
  ],
  "a fleet spans providers and each host keeps its own",
);
assert.deepEqual(
  hostsWithoutFleet(mixed).map((m) => m.node.id),
  ["b"],
  "the local agent's view cannot join a fleet",
);
assert.deepEqual(fleetTotals(fleetMembers(mixed, "f1")), { nodes: 2, online: 1, guests: 6, running: 4 });
assert.deepEqual(fleetTotals([]), { nodes: 0, online: 0, guests: 0, running: 0 });

const guests = [
  { id: "p", name: "panel", kind: "vm", status: "running", node: "", self: true },
  { id: "a", name: "a", kind: "ct", status: "Running", node: "" },
  { id: "b", name: "b", kind: "ct", status: "stopped", node: "" },
];
assert.deepEqual(
  stoppableGuests(guests).map((g) => g.id),
  ["a", "p"],
  "running only, in any case, and the panel last",
);

assert.equal(firstAnswering([node("dead", undefined, false), node("up")])?.id, "up", "a dead host does not hide a live one");
assert.equal(firstAnswering([node("dead", undefined, false)])?.id, "dead", "but it is still shown when it is all there is");
assert.equal(firstAnswering([]), undefined);

assert.equal(formatUptime(90), "1m");
assert.equal(formatUptime(7300), "2h 1m");
assert.equal(formatUptime(90000), "1d 1h");
assert.equal(memoryPercent({ mem_total_bytes: 200, mem_used_bytes: 50 }), 25);
assert.equal(memoryPercent({}), null);
assert.equal(memoryPercent(undefined), null);

assert.equal(verbApplies("start", "Running"), false, "a running guest cannot be started");
assert.equal(verbApplies("stop", "running"), true);
assert.equal(verbApplies("start", "stopped"), true);
assert.equal(verbApplies("reboot", "Stopped"), false, "a stopped guest cannot be rebooted");
assert.equal(verbApplies("shutdown", "stopped"), false);
assert.equal(verbApplies("stop", "Frozen"), true, "a status the panel does not know is left to the hypervisor");

assert.equal(noteOK("disk"), false, "too short to say anything");
assert.equal(noteOK("    disk    "), false, "spaces do not count");
assert.equal(noteOK("disk is full"), true);
assert.equal(noteOK("x".repeat(301)), false, "too long for the audit log");
assert.equal(noteOK("é".repeat(300)), true, "the limit counts characters, not bytes");

assert.equal(guestNameOK("web-1"), true);
assert.equal(guestNameOK("1web"), false, "starts with a letter");
assert.equal(guestNameOK("web_1"), false);
assert.equal(guestNameOK(""), false);
assert.deepEqual(imagesFor([{ kind: "ct" }, { kind: "vm" }, {}], "ct"), [{ kind: "ct" }, {}], "an image with no kind suits either");
assert.deepEqual(
  guestNetworks([
    { name: "vmbr0", type: "bridge" },
    { name: "vmbr1", type: "OVSBridge" },
    { name: "eth0", type: "eth" },
  ]).map((n) => n.name),
  ["vmbr0", "vmbr1"],
);

// Confirming a detection picks the provider, fills its base URL and carries the certificate to trust.
const cert = { fingerprint: "AB:CD", subject: "s", issuer: "i", names: ["pve"], not_after: "2030-01-01T00:00:00Z", pem: "PEM" };
const found = (over: Partial<HvDetection> = {}): HvDetection => ({
  address: "pve.example",
  base_url: "https://pve.example:8006",
  outcome: "recognised",
  kind: "withurl",
  version: "9.2",
  certificate: cert,
  certificate_verified: false,
  evidence: ["answered"],
  probes: [],
  ...over,
});
const withURL: InfraProvider = {
  ...provider("withurl"),
  fields: [
    { name: "base_url", required: true, label: "URL" },
    { name: "node", required: false, label: "Node", default: "pve" },
  ],
};
const tokenOnly: InfraProvider = {
  ...provider("tokenonly"),
  fields: [{ name: "trust_token", required: false, secret: true, label: "Token" }],
};
assert.deepEqual(prefillFromDetection(found(), [tokenOnly, withURL]), {
  kind: "withurl",
  values: { base_url: "https://pve.example:8006", node: "pve" },
  trusted: "PEM",
});
assert.deepEqual(
  prefillFromDetection(found({ kind: "tokenonly" }), [tokenOnly, withURL]),
  { kind: "tokenonly", values: { trust_token: "" }, trusted: "PEM" },
  "a provider without a base URL field gets none",
);
assert.equal(
  prefillFromDetection(found({ certificate_verified: true }), [withURL])?.trusted,
  "",
  "a certificate the panel already trusts is not pinned",
);
assert.equal(prefillFromDetection(found({ certificate: null }), [withURL])?.trusted, "");
assert.equal(
  prefillFromDetection(found({ outcome: "unrecognised", kind: undefined }), [withURL]),
  undefined,
  "only a recognised server is offered",
);
assert.equal(prefillFromDetection(found({ outcome: "unreachable", kind: undefined }), [withURL]), undefined);
assert.equal(
  prefillFromDetection(found({ kind: "elsewhere" }), [withURL]),
  undefined,
  "a kind this panel has no provider for is not offered",
);
assert.equal(prefillFromDetection(found(), [{ ...withURL, attachable: false }]), undefined, "nor one that cannot be attached");
assert.equal(prefillFromDetection(undefined, [withURL]), undefined);
assert.equal(sameCertificate("AB:CD:EF", "abcdef"), true);
assert.equal(sameCertificate("AB:CD:EF", "AB:CD:00"), false);
assert.equal(sameCertificate(undefined, ""), false, "no fingerprint matches nothing");

// The fleet: every host of every provider side by side, sortable, unknown never read as zero.
const GB = 1024 ** 3;
const host = (id: string, label: string, extra: Partial<InfraNode> = {}): InfraNode => ({
  id,
  label,
  kind: "k",
  source: "api",
  online: true,
  health_state: "operational",
  guests: { total: 0, running: 0 },
  ...extra,
});
const fleet = overview(
  {
    ...provider("incus"),
    label: "Incus",
    nodes: [
      host("a", "alpha", {
        details: {
          cpus: 16,
          mem_total_bytes: 64 * GB,
          mem_used_bytes: 48 * GB,
          disks: [
            { path: "/dev/sda", size_bytes: 2 * GB },
            { path: "/dev/sdb", size_bytes: 3 * GB },
          ],
        },
        guests: { total: 4, running: 4 },
        fleet_id: "f1",
        site: "lab",
      }),
      host("b", "bravo", { online: false, health_state: "down", guests: { total: 2, running: 0 } }),
    ],
  },
  {
    ...provider("proxmox"),
    label: "Proxmox",
    nodes: [
      host("a", "Charlie", { details: { cpus: 8, mem_total_bytes: 32 * GB, mem_used_bytes: 8 * GB }, guests: { total: 5, running: 3 } }),
      host("d", "delta", { health_state: "unknown", details: { mem_total_bytes: 16 * GB }, guests: { total: 1, running: 1 } }),
    ],
  },
);
fleet.fleets = [{ id: "f1", label: "Rack one" }];
const rows = fleetRows(fleet);
assert.deepEqual(
  rows.map((r) => r.id),
  ["incus:a", "incus:b", "proxmox:a", "proxmox:d"],
  "both kinds in one list; the same host id under two providers stays two rows",
);
const alpha = rows[0]!;
assert.equal(alpha.providerLabel, "Incus");
assert.equal(alpha.fleetLabel, "Rack one");
assert.equal(alpha.cpus, 16);
assert.equal(alpha.memFraction, 0.75);
assert.equal(alpha.diskBytes, 5 * GB);
assert.equal(alpha.diskCount, 2);
const bravo = rows[1]!;
assert.equal(bravo.cpus, null, "a host that reported nothing has unknowns, not zeros");
assert.equal(bravo.memFraction, null);
assert.equal(bravo.diskBytes, null);
assert.equal(rows[3]!.memUsed, null, "a total without a used figure is not an empty host");
assert.equal(rows[3]!.memFraction, null);

const order = (key: FleetSortKey, dir: "ascending" | "descending") => sortFleet(rows, key, dir).map((r) => r.node.label);
assert.deepEqual(order("name", "ascending"), ["alpha", "bravo", "Charlie", "delta"], "names sort without regard to case");
assert.deepEqual(order("name", "descending"), ["delta", "Charlie", "bravo", "alpha"]);
assert.deepEqual(order("memory", "descending"), ["alpha", "Charlie", "bravo", "delta"], "fullest first; hosts with no figure go last");
assert.deepEqual(order("memory", "ascending"), ["Charlie", "alpha", "bravo", "delta"], "and still last the other way");
assert.deepEqual(order("cpu", "descending"), ["alpha", "Charlie", "bravo", "delta"]);
assert.deepEqual(
  order("state", "ascending"),
  ["bravo", "delta", "alpha", "Charlie"],
  "trouble first: down, then unknown, then operational",
);
assert.deepEqual(order("guests", "descending"), ["alpha", "Charlie", "delta", "bravo"]);
assert.deepEqual(order("provider", "ascending"), ["alpha", "bravo", "Charlie", "delta"], "ties go by name");
assert.deepEqual(order("disk", "descending"), ["alpha", "bravo", "Charlie", "delta"]);
assert.deepEqual(order("site", "ascending")[0], "alpha", "a host with a site comes before those without");
assert.deepEqual(
  rows.map((r) => r.id),
  ["incus:a", "incus:b", "proxmox:a", "proxmox:d"],
  "sorting does not change the input",
);

assert.equal(hostHref("proxmox", "a b"), "/operator/hosts/proxmox?host=a%20b", "a host opens in its provider's tab, chosen in the URL");

// History: the charts a metrics range response fills, gaps kept.
const sampler = { state: "sampling" as const, interval_seconds: 60, last_sampled_at: null };
const range = (series: MetricRange["series"], times = ["2026-11-15T12:00:00Z", "2026-11-15T12:01:00Z", "not a time"]): MetricRange => ({
  server_id: "s",
  guest_id: "",
  from: "2026-11-15T12:00:00Z",
  to: "2026-11-15T12:03:00Z",
  step_seconds: 60,
  times,
  series,
  sampler,
});
const b = (avg: number, max = avg, samples = 1) => ({ avg, max, samples });
assert.deepEqual(historyCharts(undefined), []);
assert.deepEqual(historyCharts(range([])), [], "no metrics, no charts: the empty state shows");
assert.deepEqual(
  historyCharts(range([{ metric: "cpu", unit: "fraction", buckets: [null, null, null] }])),
  [],
  "a metric with no sample at all has no chart",
);
const charts = historyCharts(
  range([
    { metric: "cpu", unit: "fraction", buckets: [b(0.1, 0.3), null, b(1)] },
    { metric: "memory_used_bytes", unit: "bytes", buckets: [b(2 * GB), b(0, 0, 0), null] },
    { metric: "memory_total_bytes", unit: "bytes", buckets: [b(8 * GB), b(8 * GB), null] },
    { metric: "disk_used_bytes", unit: "bytes", buckets: [null, null, null] },
    { metric: "net_out_bytes_per_second", unit: "bytes_per_second", buckets: [b(100, 200), null, null] },
  ]),
);
assert.deepEqual(
  charts.map((c) => c.id),
  ["cpu", "memory", "network"],
);
assert.deepEqual(
  charts[0]!.series.map((s) => s.id),
  ["cpu.avg", "cpu.max"],
);
assert.deepEqual(
  charts[0]!.series[0]!.points.map((p) => p.value),
  [0.1, null],
  "a missing step stays null; an unreadable time is dropped",
);
assert.deepEqual(
  charts[0]!.series[1]!.points.map((p) => p.value),
  [0.3, null],
);
assert.deepEqual(
  charts[1]!.series[0]!.points.map((p) => p.value),
  [2 * GB, null],
  "a bucket with no samples is missing, not zero",
);
assert.equal(charts[1]!.ceiling, 8 * GB, "memory reads against the total");
assert.equal(charts[0]!.ceiling, undefined);
assert.equal(charts[2]!.unit, "bytes_per_second");
assert.deepEqual(
  charts[2]!.series.map((s) => s.id),
  ["net_out_bytes_per_second.avg"],
  "only the direction that was recorded",
);
assert.equal(historyCharts(range([{ metric: "load1", unit: "load", buckets: [b(0.5)] }], ["2026-11-15T12:00:00Z"]))[0]!.unit, "count");

console.log("infrastructure model ok");
