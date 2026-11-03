import assert from "node:assert/strict";
import type { InfraField, InfraOverview, InfraProvider } from "../../lib/hooks/use-infra";
import { fieldsComplete, filledValues, initialValues, isRunning, OVERVIEW_TAB, providersWithHosts, selectedTab, slugFrom, slugOK, tabHref } from "./model";

const totals = { nodes: 0, online: 0, guests: 0, running: 0 };
const provider = (kind: string, nodes = 0): InfraProvider => ({
  kind, label: kind, attachable: true, capabilities: [], verbs: [], fields: [], totals,
  nodes: Array.from({ length: nodes }, (_, i) => ({ id: `${kind}${i}`, label: "x", kind, source: "api" as const, online: true, guests: { total: 0, running: 0 } })),
});
const overview = (...providers: InfraProvider[]): InfraOverview => ({ providers, totals, unattached: [], checked_at: "" });

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

console.log("infrastructure model ok");
