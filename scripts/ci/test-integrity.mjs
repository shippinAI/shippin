#!/usr/bin/env node
// Test integrity: a failing test means the code is wrong until shown otherwise, so the fix belongs in the
// source. This check fails a range of commits that weakens existing tests without saying why:
//   - an assertion line in an existing test is removed or changed;
//   - a test file is deleted;
//   - a skip or an only is added (.skip(, .only(, xit(, xdescribe(, and t.Skip in an existing Go test).
// Adding tests and assertions never needs anything. When a test really is wrong, the commit that changes it
// carries a trailer saying why, for example:
//   Test-Change: the plan ID became a slug, so the test compared against the old number
//
// usage: node scripts/ci/test-integrity.mjs <base> <head>
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

export const TEST_FILE = /(?:_test\.go|\.(?:test|spec)\.[cm]?[jt]sx?)$/;
export const isTestFile = f => TEST_FILE.test(f.replaceAll('\\', '/'));
// A line that checks something. Removing or changing one is what weakens a test; a moved import is not.
const ASSERTION = /\b(?:t\.(?:Fatal|Fatalf|Error|Errorf|Fail|FailNow)|assert\w*|require\.\w+|expect\s*\(|toBe\w*|toEqual|toMatch\w*|toThrow\w*|toHave\w*|\.equal|deepEqual|strictEqual|throws|rejects|doesNotThrow|ok\s*\(|want|got)\b|[!=]==?|\bif\s/;
// A skip is a call that starts a statement, not those words inside a string or a comment.
const SKIP_ANY = /^(?:(?:it|test|describe|suite)\.(?:skip|only|todo)\s*\(|x(?:it|describe|test)\s*\(|(?:it|test|describe)\s*\([^)]*\{\s*(?:skip|only)\s*:\s*true)/;
const SKIP_GO = /\bt\.Skip(?:Now|f)?\s*\(/;
const TRAILER = /^Test-Change:\s*(.{15,})$/im;

const git = (args, cwd) => execFileSync('git', args, { cwd, encoding: 'utf8', maxBuffer: 256 * 1024 * 1024 });

// Problems in one file's diff. status is git's letter: A added, M modified, D deleted, R renamed.
export function problems(file, status, diff) {
  const out = [];
  if (status === 'D') return [`deletes the test file ${file}`];
  const go = file.endsWith('.go');
  for (const line of diff.split('\n')) {
    if (line.startsWith('---') || line.startsWith('+++')) continue;
    const body = line.slice(1).trim();
    if (!body || body.startsWith('//') || body.startsWith('*')) continue;
    if (line.startsWith('-') && status !== 'A' && ASSERTION.test(body)) out.push(`removes or changes a check: ${body.slice(0, 120)}`);
    if (line.startsWith('+') && (SKIP_ANY.test(body) || (go && status !== 'A' && SKIP_GO.test(body)))) out.push(`adds a skip: ${body.slice(0, 120)}`);
  }
  return out;
}

export function check(base, head, cwd = process.cwd()) {
  // Compare from where head left base, so newer work on base never reads as deleted on head.
  base = git(['merge-base', base, head], cwd).trim();
  const changed = git(['diff', '--name-status', '--no-renames', `${base}`, `${head}`], cwd).split('\n').filter(Boolean)
    .map(l => l.split('\t')).filter(([, f]) => f && isTestFile(f));
  const findings = [];
  for (const [status, file] of changed) {
    const diff = status === 'D' ? '' : git(['diff', '-U0', `${base}`, `${head}`, '--', file], cwd);
    const found = problems(file, status[0], diff);
    if (!found.length) continue;
    // Simplified path history hides a reconciliation merge when it takes one
    // parent's version. Include it, but only accept an explanation from a commit
    // that actually changes this path relative to its first parent. Otherwise an
    // unrelated later merge could excuse an earlier unexplained removal.
    const commits = git(['log', '--full-history', '--format=%H', `${base}..${head}`, '--', file], cwd).split('\n').filter(Boolean);
    const why = commits.some(c => {
      if (!TRAILER.test(git(['log', '-1', '--format=%B', c], cwd))) return false;
      return git(['diff', '--name-only', `${c}^1`, c, '--', file], cwd).trim() !== '';
    });
    if (!why) findings.push({ file, found });
  }
  return findings;
}

export const RULE = `A failing test means the code is wrong until shown otherwise: fix the source, not the test.
Do not weaken, skip or delete a test to make CI pass. If a test itself is wrong, the commit that changes it
says why in a trailer, for example:
  Test-Change: the endpoint now answers 404 for a missing host, which the old test called a 500`;

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [base, head = 'HEAD'] = process.argv.slice(2);
  if (!base) {
    console.error('usage: test-integrity.mjs <base> <head>');
    process.exit(2);
  }
  const findings = check(base, head);
  if (!findings.length) {
    console.log(`Test integrity: no test was weakened in ${base.slice(0, 12)}..${head.slice(0, 12)}.`);
    process.exit(0);
  }
  console.error('TEST INTEGRITY FAILED: existing tests were weakened without a Test-Change trailer.\n');
  for (const f of findings) {
    console.error(`${f.file}`);
    for (const p of f.found.slice(0, 12)) console.error(`  - ${p}`);
    if (f.found.length > 12) console.error(`  - and ${f.found.length - 12} more`);
  }
  console.error(`\n${RULE}`);
  process.exit(1);
}
