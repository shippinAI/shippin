#!/usr/bin/env node
// Fails the web tests when code reaches where it should not (docs/web-framework.md, rot checks):
//   - src/lib imports no feature, page, template, app or module code;
//   - a feature reaches another feature only through that feature's index.ts;
//   - nothing outside src/modules imports a module (the generated route files only name them);
//   - no folder is a bucket (common, shared, utils, util, helpers, misc): code belongs to what it serves.
//   - src/features gains no folder: a feature is a context's panel code (docs/architecture.md), and new
//     work goes inside the feature of its context. scripts/features-baseline.txt lists today's folders
//     and only shrinks, by the same two rules as the list below.
// Imports from one feature into another's internals that existed when this check arrived are listed in
// scripts/boundaries-baseline.txt. The list may only shrink: a new one fails, and so does a listed one
// that is gone (remove its line). The file is also compared with its version at the merge base, and a
// line that was not there fails, so adding the new import to the baseline does not make it pass.
// `node scripts/check-boundaries.mjs --print` prints the current list.
import { spawnSync } from "node:child_process";
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";

const at = process.argv.indexOf("--web");
const web = at >= 0 ? process.argv[at + 1] : fileURLToPath(new URL("..", import.meta.url));
const src = join(web, "src");
const baselineFile = join(web, "scripts", "boundaries-baseline.txt");
const featuresFile = join(web, "scripts", "features-baseline.txt");
const buckets = new Set(["common", "shared", "utils", "util", "helpers", "misc"]);

const posix = (p) => p.split(sep).join("/");
const problems = [];
const crossings = [];

// git runs git in cwd and returns stdout without the trailing newline, or null when it fails.
function git(cwd, args) {
  const r = spawnSync("git", args, { cwd, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] });
  if (r.status !== 0) return null;
  return r.stdout.replace(/\n$/, "");
}

function commitExists(cwd, rev) {
  return Boolean(rev) && !/^0+$/.test(rev) && git(cwd, ["cat-file", "-e", `${rev}^{commit}`]) !== null;
}

// baseRef is the commit the baseline is compared with: the pull request base, main for a queue
// branch (that is what the Gate promotes), else the push's previous tip, else main.
function baseRef(root) {
  const ref = process.env.REF || "";
  const queue = ref.startsWith("refs/heads/queue/");
  if (!queue && commitExists(root, process.env.PR_BASE || "")) return process.env.PR_BASE;
  if (queue || !commitExists(root, process.env.BEFORE || "")) {
    for (const name of ["origin/main", "main"]) if (commitExists(root, name)) return name;
    return "";
  }
  return process.env.BEFORE;
}

function baselineLines(text) {
  return text.split(/\r?\n/).filter((l) => l && !l.startsWith("#"));
}

// priorBaseline is the baseline at the merge base, when this tree is a git checkout. skip means
// there is no checkout to compare (a fixture). error means the comparison was required and could not run.
function priorBaseline(dir, file) {
  const root = git(dir, ["rev-parse", "--show-toplevel"]);
  if (root === null) return { skip: true };
  const rel = relative(root, file).split(sep).join("/");
  if (!rel || rel.startsWith("..")) return { skip: true };
  const base = baseRef(root);
  if (!base) return { error: "cannot find main to prove scripts/boundaries-baseline.txt only shrinks" };
  const mb = git(root, ["merge-base", base, "HEAD"]);
  if (mb === null) return { error: `no merge base between ${base} and HEAD; fetch the base branch` };
  const text = git(root, ["show", `${mb}:${rel}`]);
  return { lines: baselineLines(text ?? "") };
}

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
const baseline = existsSync(baselineFile) ? baselineLines(readFileSync(baselineFile, "utf8")) : [];
const listed = new Set(baseline);
for (const c of current) if (!listed.has(c)) problems.push(`${c}: reaches into another feature; import it from that feature's index.ts`);
const now = new Set(current);
for (const b of baseline) if (!now.has(b)) problems.push(`scripts/boundaries-baseline.txt lists "${b}", which is gone; remove the line`);
const prior = priorBaseline(web, baselineFile);
if (prior.error) problems.push(prior.error);
else if (!prior.skip) {
  const had = new Set(prior.lines);
  for (const b of baseline) if (!had.has(b)) problems.push(`scripts/boundaries-baseline.txt adds "${b}"; the list may only shrink`);
}

// The feature folders: only checked where the list exists, so a tree without one is not judged by it.
if (existsSync(featuresFile) && existsSync(join(src, "features"))) {
  const allowed = baselineLines(readFileSync(featuresFile, "utf8"));
  const folders = readdirSync(join(src, "features")).filter((n) => statSync(join(src, "features", n)).isDirectory());
  for (const f of folders)
    if (!allowed.includes(f))
      problems.push(`src/features/${f}: a new feature folder; put it inside the feature of its context (docs/architecture.md)`);
  for (const a of allowed)
    if (!folders.includes(a)) problems.push(`scripts/features-baseline.txt lists "${a}", which is gone; remove the line`);
  const before = priorBaseline(web, featuresFile);
  if (before.error) problems.push(before.error.replace("boundaries-baseline.txt", "features-baseline.txt"));
  else if (!before.skip && before.lines.length) {
    for (const a of allowed)
      if (!before.lines.includes(a)) problems.push(`scripts/features-baseline.txt adds "${a}"; the list may only shrink`);
  }
}

if (problems.length) {
  console.error(problems.map((p) => "boundaries: " + p).join("\n"));
  process.exit(1);
}
console.log(`check-boundaries ok (${current.length} older imports into other features, listed in the baseline)`);
