#!/usr/bin/env node
// Fails the web tests when code reaches where it should not (docs/web-framework.md, rot checks):
//   - src/lib imports no feature, page, template, app or module code;
//   - a feature reaches another feature only through that feature's index.ts;
//   - nothing outside src/modules imports a module (the generated route files only name them);
//   - no folder is a bucket (common, shared, utils, util, helpers, misc): code belongs to what it serves.
// Imports from one feature into another's internals that existed when this check arrived are listed in
// scripts/boundaries-baseline.txt. The list may only shrink: a new one fails, and so does a listed one
// that is gone (remove its line). `node scripts/check-boundaries.mjs --print` prints the current list.
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";

const at = process.argv.indexOf("--web");
const web = at >= 0 ? process.argv[at + 1] : fileURLToPath(new URL("..", import.meta.url));
const src = join(web, "src");
const baselineFile = join(web, "scripts", "boundaries-baseline.txt");
const buckets = new Set(["common", "shared", "utils", "util", "helpers", "misc"]);

const posix = (p) => p.split(sep).join("/");
const problems = [];
const crossings = [];

function walk(dir, out = []) {
  for (const name of readdirSync(dir).sort()) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) {
      if (buckets.has(name)) problems.push(`${posix(relative(web, p))}: a bucket folder; put its code with what it serves`);
      walk(p, out);
    } else if (/\.tsx?$/.test(name)) out.push(p);
  }
  return out;
}

// The source file an import names, relative to src ("" when it is a package).
function target(file, spec) {
  if (!spec.startsWith(".")) return "";
  const base = join(dirname(file), spec);
  for (const c of [base, base + ".ts", base + ".tsx", join(base, "index.ts"), join(base, "index.tsx")])
    if (existsSync(c) && statSync(c).isFile()) return posix(relative(src, c));
  return posix(relative(src, base));
}

const area = (rel) => rel.split("/")[0];
const feature = (rel) => (rel.startsWith("features/") ? rel.split("/")[1] : "");

for (const file of walk(src)) {
  const rel = posix(relative(src, file));
  const text = readFileSync(file, "utf8");
  const test = /\.test\.tsx?$/.test(rel);
  const generated = /\.gen\.ts$/.test(rel);
  for (const m of text.matchAll(/(?:\bfrom\s+|\bimport\s*\(\s*|^\s*import\s+)["']([^"']+)["']/gm)) {
    const to = target(file, m[1]);
    if (!to) continue;
    if (area(rel) === "lib" && !test && ["features", "routes", "template", "app", "modules"].includes(area(to)))
      problems.push(`${rel}: src/lib imports ${to}; lib is shared by everything and depends on none of it`);
    if (area(to) === "modules" && area(rel) !== "modules" && !generated)
      problems.push(`${rel}: imports module code ${to}; only the generated route files may name a module`);
    const from = feature(rel);
    const into = feature(to);
    if (from && into && from !== into && !/^features\/[^/]+\/index\.tsx?$/.test(to)) crossings.push(`${rel} -> ${to}`);
  }
}

const current = [...new Set(crossings)].sort();
if (process.argv.includes("--print")) {
  console.log(current.join("\n"));
  process.exit(0);
}
const baseline = existsSync(baselineFile)
  ? readFileSync(baselineFile, "utf8").split(/\r?\n/).filter((l) => l && !l.startsWith("#"))
  : [];
const listed = new Set(baseline);
for (const c of current) if (!listed.has(c)) problems.push(`${c}: reaches into another feature; import it from that feature's index.ts`);
const now = new Set(current);
for (const b of baseline) if (!now.has(b)) problems.push(`scripts/boundaries-baseline.txt lists "${b}", which is gone; remove the line`);

if (problems.length) {
  console.error(problems.map((p) => "boundaries: " + p).join("\n"));
  process.exit(1);
}
console.log(`check-boundaries ok (${current.length} older imports into other features, listed in the baseline)`);
