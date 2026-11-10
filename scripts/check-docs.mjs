// Fails when a tracked Markdown file links to a repository path that does not exist.
// Usage: node scripts/check-docs.mjs (from anywhere inside the repository).
import fs from "node:fs";
import path from "node:path";
import { execFileSync } from "node:child_process";

const root = execFileSync("git", ["rev-parse", "--show-toplevel"], { encoding: "utf8" }).trim();
// third_party keeps imported files as they were, links included.
const files = execFileSync("git", ["ls-files", "*.md", ":!third_party/"], { cwd: root, encoding: "utf8" }).trim().split("\n").filter(Boolean);
const link = /\]\(([^)\s]+)\)|^\[[^\]]+\]:\s*(\S+)/gm;
const broken = [];

for (const file of files) {
  const text = fs.readFileSync(path.join(root, file), "utf8").replace(/```[\s\S]*?```/g, "");
  for (const m of text.matchAll(link)) {
    const target = m[1] ?? m[2];
    if (/^([a-z][a-z0-9+.-]*:|#)/i.test(target)) continue;
    const local = decodeURI(target.split("#")[0]);
    if (!local) continue;
    const abs = local.startsWith("/") ? path.join(root, local) : path.join(root, path.dirname(file), local);
    if (!fs.existsSync(abs)) broken.push(`${file}: ${target}`);
  }
}

if (broken.length) {
  console.error(`Broken documentation links (${broken.length}):\n  ${broken.join("\n  ")}`);
  process.exit(1);
}
console.log(`Documentation links: ${files.length} files, all resolve.`);
