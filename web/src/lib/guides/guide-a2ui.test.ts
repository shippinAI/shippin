// Clyffy guides rendered as A2UI v0.9 (jiti runs the TS).
import { test } from "node:test";
import assert from "node:assert/strict";
import { terminalGuides } from "../../features/clyffy/guides/all-guides";
import { applyAll } from "../a2ui/a2ui-model";
import type { A2UIComponent } from "../a2ui/a2ui-types";
import { guideMessages, pickerMessages } from "./guide-a2ui";

/** Every referenced child id exists (A2UI validators check the adjacency list the same way). */
function refsResolve(c: Record<string, A2UIComponent>) {
  for (const x of Object.values(c)) {
    const kids = "children" in x && Array.isArray(x.children) ? x.children : "child" in x ? [x.child] : [];
    for (const k of kids) assert.ok(c[k], `${x.id} -> ${k}`);
  }
}

test("picker: a List templated over /guides with one item per guide", () => {
  const s = applyAll(pickerMessages(terminalGuides))["node-picker"];
  assert.ok(s.components.root);
  refsResolve(s.components);
  assert.equal((s.data as { guides: unknown[] }).guides.length, terminalGuides.length);
});

test("each guide renders the active step card with run/next actions and no dangling refs", () => {
  for (const g of terminalGuides) {
    const s = applyAll(guideMessages(g, 0, {}))[`node-${g.id}`];
    assert.ok(s.components.root, g.id);
    refsResolve(s.components);
    const names = Object.values(s.components).flatMap((c) => (c.component === "Button" && "event" in c.action ? [c.action.event.name] : []));
    if (g.steps[0].command) assert.ok(names.includes("run"), g.id);
    if (g.steps.length > 1) assert.ok(names.includes("next") && names.includes("goto"), g.id);
  }
});

test("'What happened' shows only after the step ran", () => {
  const g = terminalGuides.find((x) => x.steps[0].command)!;
  const before = applyAll(guideMessages(g, 0, {}))[`node-${g.id}`].components;
  const after = applyAll(guideMessages(g, 0, { [g.steps[0].id]: "ran" }))[`node-${g.id}`].components;
  assert.equal(before["s0-explain"], undefined);
  assert.ok(after["s0-explain"]);
});
