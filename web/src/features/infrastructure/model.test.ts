import assert from "node:assert/strict";
import type { InfraField, InfraOverview, InfraProvider } from "../../lib/hooks/use-infra";
import { verbApplies, firstAnswering, FLEETS_TAB, fieldsComplete, filledValues, fleetMembers, fleetTotals, formatUptime, hostsWithoutFleet, initialValues, isRunning, memoryPercent, OVERVIEW_TAB, providersWithHosts, selectedTab, slugFrom, slugOK, stoppableGuests, tabHref } from "./model";

const totals = { nodes: 0, online: 0, guests: 0, running: 0 };
const provider = (kind: string, nodes = 0): InfraProvider => ({
  kind, label: kind, attachable: true, capabilities: [], verbs: [], fields: [], totals,
  nodes: Array.from({ length: nodes }, (_, i) => ({ id: `${kind}${i}`, label: "x", kind, source: "api" as const, online: true, guests: { total: 0, running: 0 } })),
});
const overview = (...providers: InfraProvider[]): InfraOverview => ({ providers, totals, fleets: [], unattached: [], checked_at: "" });

const none = overview(provider("first"), provider("second"));
assert.deepEqual(providersWithHosts(none), [], "no host, no tab");
assert.deepEqual(providersWithHosts(undefined), []);

const one = overview(provider("first", 1), provider("second"));
assert.deepEqual(providersWithHosts(one).map((p) => p.kind), ["first"], "adding a host adds that provider's tab");
const both = overview(provider("first", 1), provider("second", 2));
assert.deepEqual(providersWithHosts(both).map((p) => p.kind), ["first", "second"]);

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

const node = (id: string, fleet_id?: string, online = true, source: "api" | "agent" = "api") => ({ id, label: id, kind: "k", source, online, fleet_id, guests: { total: 3, running: 2 } });
const mixed = overview(
  { ...provider("first"), label: "First", nodes: [node("a", "f1"), node("b"), node("local", undefined, true, "agent")] },
  { ...provider("second"), label: "Second", nodes: [node("c", "f1", false)] },
);
assert.deepEqual(fleetMembers(mixed, "f1").map((m) => [m.node.id, m.providerLabel]), [["a", "First"], ["c", "Second"]], "a fleet spans providers and each host keeps its own");
assert.deepEqual(hostsWithoutFleet(mixed).map((m) => m.node.id), ["b"], "the local agent's view cannot join a fleet");
assert.deepEqual(fleetTotals(fleetMembers(mixed, "f1")), { nodes: 2, online: 1, guests: 6, running: 4 });
assert.deepEqual(fleetTotals([]), { nodes: 0, online: 0, guests: 0, running: 0 });

const guests = [
  { id: "p", name: "panel", kind: "vm", status: "running", node: "", self: true },
  { id: "a", name: "a", kind: "ct", status: "Running", node: "" },
  { id: "b", name: "b", kind: "ct", status: "stopped", node: "" },
];
assert.deepEqual(stoppableGuests(guests).map((g) => g.id), ["a", "p"], "running only, in any case, and the panel last");

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

console.log("infrastructure model ok");
