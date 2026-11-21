#!/usr/bin/env node
// knip over the web code (web/knip.json): files, exports, types and dependencies nothing uses. Findings that
// existed when the check arrived are in scripts/unused-baseline.txt, which may only shrink.
// Two development dependencies are not visible to knip and are ignored there: @biomejs/biome (run as a
// command by scripts/test.mjs and check-lint.mjs) and tailwindcss-react-aria-components (a Tailwind plugin
// loaded by src/styles/app.css).
// Usage: node scripts/check-unused.mjs [--update]
import { spawnSync } from "node:child_process";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { ratchet } from "./baseline.mjs";

const web = fileURLToPath(new URL("..", import.meta.url));
const bin = join(web, "node_modules", ".bin", process.platform === "win32" ? "knip.cmd" : "knip");
const r = spawnSync(bin, ["--no-progress", "--reporter", "json"], {
  cwd: web,
  encoding: "utf8",
  maxBuffer: 64 << 20,
  shell: process.platform === "win32",
});
let report;
try {
  report = JSON.parse(r.stdout);
} catch {
  console.error(`check-unused: knip did not report:\n${r.stdout}${r.stderr}`);
  process.exit(1);
}

const kinds = {
  files: "file",
  dependencies: "dependency",
  devDependencies: "devDependency",
  unlisted: "unlisted dependency",
  unresolved: "unresolved import",
  exports: "export",
  types: "type",
  enumMembers: "enum member",
  duplicates: "duplicate export",
};
const found = new Map();
for (const issue of report.issues ?? []) {
  for (const [field, kind] of Object.entries(kinds)) {
    for (const item of issue[field] ?? []) {
      const name = Array.isArray(item) ? item.map((x) => x.name).join("|") : item.name;
      const key = `${issue.file} ${kind} ${name}`;
      found.set(key, (found.get(key) ?? 0) + 1);
    }
  }
}
const header = [
  "What knip found unused when check-unused arrived (2026-11-20).",
  "This list may only shrink: remove the unused code, then run `node scripts/check-unused.mjs --update`.",
];
const problems = ratchet(join(web, "scripts", "unused-baseline.txt"), header, found, {
  update: process.argv.includes("--update"),
  fix: "remove it or use it (`npx knip` shows where)",
});
if (problems.length) {
  console.error(problems.map((p) => `check-unused: ${p}`).join("\n"));
  process.exit(1);
}
console.log(`check-unused ok (${found.size} older findings in the baseline)`);
