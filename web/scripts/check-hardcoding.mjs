#!/usr/bin/env node
// Fails the web tests when the panel names something only one installation has. Names and labels
// belong in config (ui.* in shippin.json, or the Interface page), so a new site never sees another's.
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";

const src = fileURLToPath(new URL("../src", import.meta.url));

// The infrastructure pages learn every provider from the API. A provider's name in them means a
// new driver would need a panel change, and would break a site that renamed it.
const providerNames = /\b(incus|proxmox|pve|lxc|qemu)\b/i;
const providerFree = [/^features\/infrastructure\//, /^lib\/ui-config\//, /^lib\/hooks\/use-infra\.ts$/, /^features\/admin\/ui\/interface\//];

function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) walk(p, out);
    else if (/\.tsx?$/.test(name) && !/\.test\.tsx?$/.test(name)) out.push(p);
  }
  return out;
}

const problems = [];
for (const file of walk(src)) {
  const rel = relative(src, file).split(sep).join("/");
  const lines = readFileSync(file, "utf8").split(/\r?\n/);
  lines.forEach((line, i) => {
    const at = `${rel}:${i + 1}`;
    if (providerNames.test(line) && providerFree.some((re) => re.test(rel))) problems.push(`${at}: names a provider; it should come from the API: ${line.trim().slice(0, 100)}`);
  });
}
if (problems.length) {
  console.error(`check-hardcoding: ${problems.length} problem(s)\n${problems.join("\n")}`);
  process.exit(1);
}
console.log("check-hardcoding ok");
