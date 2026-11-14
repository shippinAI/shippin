import assert from "node:assert/strict";
import { test } from "node:test";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { execFileSync, spawnSync } from "node:child_process";

const checker = fileURLToPath(new URL("./check-docs.mjs", import.meta.url));
function check(t, markdown) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "shippin-doc-links-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  execFileSync("git", ["init", "--quiet", root]);
  fs.writeFileSync(path.join(root, "README.md"), markdown);
  fs.writeFileSync(path.join(root, "exists.md"), "# Exists\n");
  execFileSync("git", ["add", "README.md", "exists.md"], { cwd: root });
  return spawnSync(process.execPath, [checker], { cwd: root, encoding: "utf8" });
}

test("actual checker ignores inline generic calls and code examples", t => {
  const result = check(t, 'Use `mcpl.Need[T](c)` and ``a ` [fake](missing.md)``.\n\n```go\n[fake](missing.md)\n```\n[real](exists.md)');
  assert.equal(result.status, 0, result.stderr);
});
test("a real missing link next to code still fails", t => {
  const result = check(t, '`mcpl.Need[T](c)` [broken](missing.md)');
  assert.equal(result.status, 1);
  assert.match(result.stderr, /README.md: missing.md/);
  assert.doesNotMatch(result.stderr, /README.md: c\b/);
});
test("code in a link label does not hide the destination", t => {
  const result = check(t, '[`code`](missing.md)');
  assert.equal(result.status, 1);
  assert.match(result.stderr, /missing.md/);
});
test("unmatched and escaped backticks do not suppress real links", t => {
  const result = check(t, '`unclosed\n\n[one](missing-one.md)\n\\` [two](missing-two.md) \\`');
  assert.equal(result.status, 1);
  assert.match(result.stderr, /missing-one.md/);
  assert.match(result.stderr, /missing-two.md/);
});
test("reference definitions, root paths, anchors and external links retain behavior", t => {
  const result = check(t, '[one](/exists.md#heading) [two](https://example.com) [three](#local)\n[ref]: exists.md\n[bad]: missing.md');
  assert.equal(result.status, 1);
  assert.match(result.stderr, /Broken documentation links \(1\)/);
  assert.match(result.stderr, /README.md: missing.md/);
});
