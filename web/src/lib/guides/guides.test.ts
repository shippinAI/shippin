// Guided experiences, the terminal bus and expiry labels (jiti runs the TS).
import { test } from "node:test";
import assert from "node:assert/strict";
import { terminalGuides } from "../../features/clyffy/guides/all-guides";
import { expiresIn } from "../expiry";
import { TerminalBus } from "../terminal/terminal-bus";
import { fillGuide } from "./guide-types";

test("every guide has unique step ids and an explanation per step", () => {
  assert.deepEqual(terminalGuides.map((g) => g.id), ["connect-accounts", "clone-repo", "adopt-folder", "publish-project"]);
  for (const g of terminalGuides) {
    const ids = g.steps.map((s) => s.id);
    assert.equal(new Set(ids).size, ids.length, g.id);
    for (const s of g.steps) assert.ok(s.explain && (s.command || s.link), `${g.id}/${s.id}`);
  }
});

test("fillGuide resolves page links and leaves unknown tokens", () => {
  const g = fillGuide(terminalGuides[0], { connectUrl: "/t/a/workspaces/b/connect" });
  assert.equal(g.steps[0].link?.to, "/t/a/workspaces/b/connect");
  assert.equal(fillGuide({ id: "x", title: "", summary: "", steps: [{ id: "s", title: "", body: "{nope}", explain: "" }] }, {}).steps[0].body, "{nope}");
});

test("terminal bus queues until attached and presses Enter unless editing", () => {
  const bus = new TerminalBus();
  const got: string[] = [];
  bus.type("gh auth status");
  bus.type("gh repo clone OWNER/REPO", false);
  bus.attach((d) => got.push(d));
  bus.type("ls");
  assert.deepEqual(got, ["gh auth status\r", "gh repo clone OWNER/REPO", "ls\r"]);
});

test("expiresIn", () => {
  const now = Date.parse("2026-10-19T00:00:00Z");
  assert.equal(expiresIn("2026-10-22T01:00:00Z", now), "Expires in 3d");
  assert.equal(expiresIn("2026-10-19T05:30:00Z", now), "Expires in 5h");
  assert.equal(expiresIn("2026-10-18T00:00:00Z", now), "Expired");
});
