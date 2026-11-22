import { gzipSync } from "node:zlib";
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";

const dist = "dist";
const limit = 50 * 1000;
let failed = false;
const htmlFiles = [];
function walk(dir) {
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) walk(path);
    else if (name.endsWith(".html")) htmlFiles.push(path);
  }
}
walk(dist);
for (const file of htmlFiles) {
  const html = readFileSync(file, "utf8");
  const srcs = [...html.matchAll(/<script[^>]+src="([^"]+)"/g)].map((m) => m[1]);
  let total = 0;
  for (const src of srcs) {
    if (/^https?:/i.test(src)) {
      console.log(`FAIL ${file} third-party script ${src}`);
      failed = true;
      continue;
    }
    const rel = src.replace(/^\//, "");
    const bytes = gzipSync(readFileSync(join(dist, rel))).length;
    total += bytes;
    console.log(`info ${file} ${src} ${(bytes / 1000).toFixed(1)} KB gz`);
  }
  const ok = total <= limit;
  failed ||= !ok;
  console.log(`${ok ? "ok  " : "FAIL"} ${file} initial JS ${(total / 1000).toFixed(1)} KB gz (budget 50)`);
}
if (failed) process.exit(1);
