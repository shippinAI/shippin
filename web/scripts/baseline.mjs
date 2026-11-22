// A baseline of findings that existed when a check arrived, which may only shrink (docs/web-framework.md).
// Each line is "<count> <finding>". A check fails on a finding above its count, and on a count above what
// is found, so a fix is recorded by lowering the line (`--update` lowers counts; it never raises one or
// adds a finding).
import { existsSync, readFileSync, writeFileSync } from "node:fs";

export function readBaseline(file) {
  const out = new Map();
  if (!existsSync(file)) return out;
  for (const line of readFileSync(file, "utf8").split(/\r?\n/)) {
    const m = /^(\d+) (.+)$/.exec(line);
    if (m) out.set(m[2], Number(m[1]));
  }
  return out;
}

/** Compares found (finding -> count) with the baseline file and returns the problems, or lowers it. */
export function ratchet(file, header, found, { update = false, fix = "fix it" } = {}) {
  const base = readBaseline(file);
  const problems = [];
  for (const [k, n] of found) {
    const allowed = base.get(k) ?? 0;
    if (n > allowed) problems.push(`${k}: ${n - allowed} new; ${fix}`);
  }
  const lowered = new Map();
  for (const [k, n] of base) {
    const now = found.get(k) ?? 0;
    if (now < n) problems.push(`${k}: ${n - now} fewer than the baseline lists; run with --update to lower it`);
    lowered.set(k, Math.min(n, now));
  }
  if (update) {
    const lines = [...lowered].filter(([, n]) => n > 0).sort(([a], [b]) => (a < b ? -1 : 1));
    writeFileSync(file, `${header.map((h) => `# ${h}`).join("\n")}\n${lines.map(([k, n]) => `${n} ${k}`).join("\n")}\n`);
    return problems.filter((p) => p.includes(" new; "));
  }
  return problems;
}

/** Writes a first baseline from found. Only for a check's first commit. */
export function writeBaseline(file, header, found) {
  const lines = [...found].sort(([a], [b]) => (a < b ? -1 : 1));
  writeFileSync(file, `${header.map((h) => `# ${h}`).join("\n")}\n${lines.map(([k, n]) => `${n} ${k}`).join("\n")}\n`);
}
