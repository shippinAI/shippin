#!/usr/bin/env node
// `npm test`: the checks (formatting included), then every test, found by name so none can be left out
// (docs/web-framework.md).
//   src/**/*.test.ts       run one by one with jiti, each in its own process
//   scripts/**/*.test.mjs  run with node --test
import { spawnSync } from "node:child_process";
import { readdirSync, statSync } from "node:fs";
import { join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";

const web = fileURLToPath(new URL("..", import.meta.url));

function find(dir, pattern, out = []) {
  for (const name of readdirSync(dir).sort()) {
    if (name === "node_modules" || name === "dist") continue;
    const p = join(dir, name);
    if (statSync(p).isDirectory()) find(p, pattern, out);
    else if (pattern.test(name)) out.push(relative(web, p).split(sep).join("/"));
  }
  return out;
}

const steps = [
  ["node", ["scripts/routes.mjs", "--check"]],
  ["node", ["scripts/generate-openapi-index.mjs", "--check"]],
  ["node", ["scripts/check-hardcoding.mjs"]],
  ["node", ["scripts/check-boundaries.mjs"]],
  ["biome", ["format"]],
  ...find(join(web, "src"), /\.test\.ts$/).map((f) => ["jiti", [f]]),
  ["node", ["--test", ...find(join(web, "scripts"), /\.test\.mjs$/)]],
];

const bin = (name) => join(web, "node_modules", ".bin", process.platform === "win32" ? `${name}.cmd` : name);
const failed = [];
for (const [cmd, args] of steps) {
  const label = [cmd, ...args].join(" ");
  const tool = cmd !== "node";
  const r = spawnSync(tool ? bin(cmd) : process.execPath, args, {
    cwd: web,
    stdio: "inherit",
    shell: tool && process.platform === "win32",
  });
  if (r.status !== 0) {
    failed.push(label);
    console.error(`FAIL ${label}`);
  }
}
const tests = steps.filter(([c]) => c === "jiti").length;
if (failed.length) {
  console.error(`\n${failed.length} of ${steps.length} steps failed:\n  ${failed.join("\n  ")}`);
  process.exit(1);
}
console.log(`\nAll ${steps.length} steps passed: ${tests} test files under src, the scripts' tests and the checks.`);
