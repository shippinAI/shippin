#!/usr/bin/env node
// Biome's recommended lint rules over the web code (web/biome.json). Errors and warnings that existed when
// the check arrived are counted per file and rule in scripts/lint-baseline.txt, which may only shrink.
// Usage: node scripts/check-lint.mjs [--update]
import { spawnSync } from "node:child_process";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { ratchet } from "./baseline.mjs";

const web = fileURLToPath(new URL("..", import.meta.url));
const bin = join(web, "node_modules", ".bin", process.platform === "win32" ? "biome.cmd" : "biome");
const r = spawnSync(bin, ["lint", "--reporter=json", "--max-diagnostics=none"], {
  cwd: web,
  encoding: "utf8",
  maxBuffer: 64 << 20,
  shell: process.platform === "win32",
});
let report;
try {
  report = JSON.parse(r.stdout);
} catch {
  console.error(`check-lint: biome did not report:\n${r.stdout}${r.stderr}`);
  process.exit(1);
}

const found = new Map();
for (const d of report.diagnostics ?? []) {
  if (d.severity !== "error" && d.severity !== "warning") continue;
  const key = `${d.location.path.replaceAll("\\", "/")} ${d.category}`;
  found.set(key, (found.get(key) ?? 0) + 1);
}
const header = [
  "Biome lint errors and warnings that existed when check-lint arrived (2026-11-20), per file and rule.",
  "This list may only shrink: fix one, then run `node scripts/check-lint.mjs --update` to lower its count.",
];
const problems = ratchet(join(web, "scripts", "lint-baseline.txt"), header, found, {
  update: process.argv.includes("--update"),
  fix: "see `npx biome lint <file>`",
});
if (problems.length) {
  console.error(problems.map((p) => `check-lint: ${p}`).join("\n"));
  process.exit(1);
}
const total = [...found.values()].reduce((a, b) => a + b, 0);
console.log(`check-lint ok (${total} older findings in the baseline)`);
