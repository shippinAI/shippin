// Bundle budget gate (CONSOLE-IA): initial JS per HTML entry and every other JS chunk, gzip.
// Usage: node scripts/check-budget.mjs [distDir]. Exits 1 on a breach; run by `npm run budget`
// and by deploy/release/build.sh, so an over-budget build cannot become a release.
import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { gzipSync } from "node:zlib";

const dist = process.argv[2] ?? "dist";
const budget = JSON.parse(readFileSync(new URL("../budget.json", import.meta.url), "utf8"));
const kb = (n) => (n / 1000).toFixed(1);
const gz = (file) => gzipSync(readFileSync(join(dist, file))).length;
const sizes = new Map(
  readdirSync(join(dist, "assets")).filter((f) => f.endsWith(".js")).map((f) => [`assets/${f}`, gz(`assets/${f}`)]),
);

let failed = false;
const initial = new Set();
for (const entry of budget.entries) {
  const html = readFileSync(join(dist, entry), "utf8");
  const files = [...html.matchAll(/(?:src|href)="\/?(assets\/[^"]+\.js)"/g)].map((m) => m[1]);
  const total = files.reduce((s, f) => s + (sizes.get(f) ?? 0), 0);
  files.forEach((f) => initial.add(f));
  const ok = total <= budget.initial_js_gz_kb * 1000;
  failed ||= !ok;
  console.log(`${ok ? "ok  " : "FAIL"} ${entry} initial JS ${kb(total)} KB gz (budget ${budget.initial_js_gz_kb}) over ${files.length} files`);
}
for (const [file, size] of [...sizes].sort((a, b) => b[1] - a[1])) {
  const cap = (budget.chunk_overrides ?? []).find((o) => file.startsWith(o.match))?.gz_kb ?? budget.route_chunk_gz_kb;
  if (initial.has(file) || size <= cap * 1000) continue;
  failed = true;
  console.log(`FAIL ${file} ${kb(size)} KB gz (route chunk budget ${cap})`);
}
const largest = [...sizes].filter(([f]) => !initial.has(f)).sort((a, b) => b[1] - a[1])[0];
if (largest) console.log(`info largest route chunk ${largest[0]} ${kb(largest[1])} KB gz`);
process.exit(failed ? 1 : 0);
