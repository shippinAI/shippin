#!/usr/bin/env node
// Runs the console tests with the database tests actually executing.
// `go test` skips every test that needs SHIPPIN_TEST_DATABASE_URL when that variable is unset, and a
// green run then hides them. The Checks workflow is what the Gate requires before main moves, so this
// command refuses to start without the URL and fails if a test still skips for that reason.
// Packages share one disposable database and rebuild its schema, so they run one at a time (-p 1).
//
// usage: node scripts/ci/database-tests.mjs
import { spawn } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

export const DATABASE_SKIP = /SHIPPIN_TEST_DATABASE_URL/;

// tally reads `go test -json` events. A database skip is a skipped test whose output names the URL.
export function tally(events) {
  const tests = { pass: 0, fail: 0, skip: 0 };
  const packages = { pass: 0, fail: 0, skip: 0 };
  const reason = new Map();
  for (const e of events) {
    const bucket = e.Test ? tests : packages;
    if (e.Action === "pass" || e.Action === "fail" || e.Action === "skip") bucket[e.Action]++;
    if (e.Action === "output" && e.Test && DATABASE_SKIP.test(e.Output || "")) {
      reason.set(`${e.Package}\t${e.Test}`, String(e.Output).trim());
    }
  }
  const databaseSkips = [];
  for (const e of events) {
    if (e.Action !== "skip" || !e.Test) continue;
    const why = reason.get(`${e.Package}\t${e.Test}`);
    if (why) databaseSkips.push({ package: e.Package, test: e.Test, output: why });
  }
  return { tests, packages, databaseSkips };
}

export function summarize(t) {
  return `database-tests: ${t.databaseSkips.length} skipped for SHIPPIN_TEST_DATABASE_URL; tests ${t.tests.pass} passed, ${t.tests.fail} failed, ${t.tests.skip} skipped; packages ${t.packages.pass} passed, ${t.packages.fail} failed, ${t.packages.skip} skipped`;
}

function main() {
  if (!process.env.SHIPPIN_TEST_DATABASE_URL) {
    console.error("SHIPPIN_TEST_DATABASE_URL is required. Database tests skip when it is unset, and this job must run them.");
    process.exit(1);
  }
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "..");
  const child = spawn("go", ["test", "-p", "1", "-count=1", "-json", "./..."], {
    cwd: root,
    stdio: ["inherit", "pipe", "inherit"],
  });
  let buf = "";
  const events = [];
  const take = (line) => {
    if (!line) return;
    let event;
    try {
      event = JSON.parse(line);
    } catch {
      process.stdout.write(`${line}\n`);
      return;
    }
    events.push(event);
    if (event.Action === "output" && event.Output) process.stdout.write(event.Output);
  };
  child.stdout.on("data", (chunk) => {
    buf += chunk.toString();
    let nl;
    while ((nl = buf.indexOf("\n")) >= 0) {
      take(buf.slice(0, nl));
      buf = buf.slice(nl + 1);
    }
  });
  child.on("close", (code) => {
    if (buf.trim()) take(buf);
    const result = tally(events);
    console.log(summarize(result));
    if (result.databaseSkips.length) {
      console.error("database tests skipped; they must run against Postgres:");
      for (const s of result.databaseSkips.slice(0, 30)) console.error(`  ${s.package} ${s.test}: ${s.output}`);
      process.exit(1);
    }
    process.exit(code ?? 1);
  });
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) main();
