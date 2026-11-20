import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { ratchet, readBaseline } from "./baseline.mjs";

const file = (text) => {
  const f = join(mkdtempSync(join(tmpdir(), "baseline-")), "b.txt");
  writeFileSync(f, text);
  return f;
};

test("a finding above its count fails, one within it passes", () => {
  const f = file("# h\n2 a.ts rule\n");
  assert.deepEqual(ratchet(f, ["h"], new Map([["a.ts rule", 2]])), []);
  assert.match(ratchet(f, ["h"], new Map([["a.ts rule", 3]]))[0], /a\.ts rule: 1 new/);
  assert.match(ratchet(f, ["h"], new Map([["b.ts rule", 1]]))[0], /b\.ts rule: 1 new/);
});

test("a fixed finding fails until the baseline is lowered, and --update only lowers", () => {
  const f = file("# h\n2 a.ts rule\n1 c.ts rule\n");
  assert.match(
    ratchet(
      f,
      ["h"],
      new Map([
        ["a.ts rule", 1],
        ["c.ts rule", 1],
      ]),
    )[0],
    /1 fewer than the baseline lists/,
  );
  assert.deepEqual(ratchet(f, ["h"], new Map([["a.ts rule", 1]]), { update: true }), []);
  assert.deepEqual([...readBaseline(f)], [["a.ts rule", 1]]);
  // A new finding is never written into the baseline.
  assert.equal(
    ratchet(
      f,
      ["h"],
      new Map([
        ["a.ts rule", 1],
        ["n.ts rule", 1],
      ]),
      { update: true },
    ).length,
    1,
  );
  assert.doesNotMatch(readFileSync(f, "utf8"), /n\.ts/);
});
