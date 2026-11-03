import assert from "node:assert/strict";
import { applyNav, compactUi, format, textFor } from "./model";

const items = [
  { id: "a", label: "A", extra: 1 },
  { id: "b", label: "B", extra: 2 },
  { id: "c", label: "C", extra: 3 },
];

assert.deepEqual(applyNav(items), items, "no overrides keep the built-in list");
assert.deepEqual(applyNav(items, [{ id: "b", label: "Renamed" }]).map((i) => i.label), ["A", "Renamed", "C"]);
assert.deepEqual(applyNav(items, [{ id: "b", hidden: true }]).map((i) => i.id), ["a", "c"]);
assert.deepEqual(applyNav(items, [{ id: "c", order: 1 }]).map((i) => i.id), ["a", "c", "b"], "ties keep built-in order");
assert.deepEqual(applyNav(items, [{ id: "a", order: 9 }]).map((i) => i.id), ["b", "c", "a"]);
assert.deepEqual(applyNav(items, [{ id: "unknown", label: "X" }]).map((i) => i.id), ["a", "b", "c"], "an id from another version is ignored");
assert.equal(applyNav(items, [{ id: "b", label: "" }])[1]!.label, "B", "an empty label keeps the default");
assert.equal(applyNav(items, [{ id: "b", label: "R" }])[1]!.extra, 2, "other fields survive");
assert.equal(items[1]!.label, "B", "the built-in list is not changed");

assert.equal(format("{a} of {b}", { a: 1, b: 2 }), "1 of 2");
assert.equal(format("{a} and {missing}", { a: 1 }), "1 and {missing}", "a typo in an override stays visible");

assert.equal(textFor(undefined, "infra.title"), "Hosts & nodes");
assert.equal(textFor({ text: { "infra.title": "Nodes" } }, "infra.title"), "Nodes");
assert.equal(textFor({ text: { "infra.title": "" } }, "infra.title"), "Hosts & nodes", "an empty override keeps the default");
assert.equal(textFor({}, "infra.provider.add", { label: "X" }), "Add X");

const compact = compactUi({
  nav: { items: [{ id: "keep", label: " Nodes " }, { id: "drop", label: "  " }, { id: "hide", hidden: true }, { id: "move", order: 2 }], sections: [{ id: "none" }] },
  text: { "a.b": " x ", "c.d": "   " },
  providers: [{ kind: "drop", label: "", fields: [{ name: "f", label: "" }] }, { kind: "keep", label: "L", fields: [{ name: "f", default: " v " }, { name: "g" }] }],
  default_site: "  ",
  drivers: [{ kind: "x", fields: [], capabilities: [], verbs: [] }],
});
assert.deepEqual(compact.nav?.items, [{ id: "keep", label: "Nodes" }, { id: "hide", hidden: true }, { id: "move", order: 2 }]);
assert.deepEqual(compact.nav?.sections, []);
assert.deepEqual(compact.text, { "a.b": "x" });
assert.deepEqual(compact.providers, [{ kind: "keep", label: "L", fields: [{ name: "f", default: "v" }] }]);
assert.equal(compact.default_site, undefined);
assert.equal((compact as { drivers?: unknown }).drivers, undefined, "driver facts are not stored as configuration");

console.log("ui-config model ok");
