import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync, spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { check, problems, isTestFile } from './test-integrity.mjs';
import { redBranches, pushedRef, pushDir, inMain } from './agent-hooks.mjs';

const here = path.dirname(fileURLToPath(import.meta.url));

function repo() {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'integrity-'));
  const git = (...a) => execFileSync('git', a, { cwd: dir, encoding: 'utf8' });
  git('init', '-q', '-b', 'main');
  git('config', 'user.email', 'test@example.invalid');
  git('config', 'user.name', 'Test');
  const write = (f, s) => { fs.mkdirSync(path.dirname(path.join(dir, f)), { recursive: true }); fs.writeFileSync(path.join(dir, f), s); };
  const commit = msg => { git('add', '-A'); git('commit', '-q', '-m', msg); return git('rev-parse', 'HEAD').trim(); };
  return { dir, git, write, commit };
}
const goTest = checks => `package x\n\nimport "testing"\n\nfunc TestX(t *testing.T) {\n${checks}\n}\n`;

test('test files are recognised by name', () => {
  for (const f of ['a/b_test.go', 'x.test.ts', 'x.test.mjs', 'y.spec.tsx', 'w\\z.test.js']) assert.ok(isTestFile(f), f);
  for (const f of ['a/b.go', 'test.ts', 'testdata/x.json', 'contest.go']) assert.ok(!isTestFile(f), f);
});

test('adding tests passes; weakening one fails until a commit says why', () => {
  const r = repo();
  r.write('x_test.go', goTest('\tif got() != 2 {\n\t\tt.Fatal("want 2")\n\t}'));
  const base = r.commit('base');
  r.write('x_test.go', goTest('\tif got() != 2 {\n\t\tt.Fatal("want 2")\n\t}\n\tif got() == 3 {\n\t\tt.Fatal("never 3")\n\t}'));
  r.write('new.test.ts', 'import assert from "node:assert";\nassert.equal(1, 1);\n');
  const added = r.commit('more tests');
  assert.deepEqual(check(base, added, r.dir), [], 'only additions');

  r.write('x_test.go', goTest('\t_ = got()'));
  const weakened = r.commit('make it pass');
  const found = check(added, weakened, r.dir);
  assert.equal(found.length, 1);
  assert.match(found[0].found.join('\n'), /removes or changes a check/);

  r.git('commit', '-q', '--amend', '-m', 'make it pass\n\nTest-Change: got() was renamed and the check moved to y_test.go');
  assert.deepEqual(check(added, r.git('rev-parse', 'HEAD').trim(), r.dir), [], 'a stated reason covers it');

  r.git('commit', '-q', '--amend', '-m', 'make it pass\n\nTest-Change: short');
  assert.equal(check(added, r.git('rev-parse', 'HEAD').trim(), r.dir).length, 1, 'a reason must say something');
});

test('skips, onlys and deleted test files are caught', () => {
  assert.match(problems('a.test.ts', 'M', '+it.skip("x", () => {})').join(), /adds a skip/);
  assert.match(problems('a.test.ts', 'A', '+test.only("x", () => {})').join(), /adds a skip/);
  assert.match(problems('a_test.go', 'M', '+\tt.Skip("flaky")').join(), /adds a skip/);
  assert.deepEqual(problems('a_test.go', 'A', '+\tt.Skip("needs SHIPPIN_RIG")'), [], 'a new test may skip without its rig');
  assert.deepEqual(problems('a.test.ts', 'M', '-import { a } from "./a";\n+import { a, b } from "./a";'), [], 'a moved import is not a check');
  assert.match(problems('a_test.go', 'D', '').join(), /deletes the test file/);
  assert.deepEqual(problems('a.test.ts', 'M', `+assert.match(f('+it.skip("x")'), /skip/);`), [], 'the words inside a string are not a skip');
  assert.match(problems('a.test.ts', 'M', '+  describe.only("x", () => {})').join(), /adds a skip/, 'indented still counts');
});

test('a branch that started before newer work on main is compared from where it left', () => {
  const r = repo();
  r.write('x_test.go', goTest('\tt.Fatal("a")'));
  r.commit('base');
  r.git('checkout', '-q', '-b', 'side');
  r.write('side.go', 'package x\n');
  const side = r.commit('side');
  r.git('checkout', '-q', 'main');
  r.write('later_test.go', goTest('\tt.Fatal("later")'));
  const main = r.commit('later test on main');
  assert.deepEqual(check(main, side, r.dir), [], 'main\'s newer test is not "deleted" on the side branch');
});

test('the CLI fails loudly and names the rule', () => {
  const r = repo();
  r.write('x_test.go', goTest('\tt.Fatal("a")'));
  const base = r.commit('base');
  fs.rmSync(path.join(r.dir, 'x_test.go'));
  const head = r.commit('drop it');
  const res = spawnSync(process.execPath, [path.join(here, 'test-integrity.mjs'), base, head], { cwd: r.dir, encoding: 'utf8' });
  assert.equal(res.status, 1);
  assert.match(res.stderr, /TEST INTEGRITY FAILED/);
  assert.match(res.stderr, /fix the source, not the test/);
});

test('a reconciliation merge can explain a test removal hidden by simplified history', () => {
  const r = repo();
  r.write('x_test.go', goTest('\tt.Fatal("a")'));
  r.commit('base');
  r.git('checkout', '-q', '-b', 'side');
  r.write('side.go', 'package x\n');
  const side = r.commit('side');
  r.git('checkout', '-q', 'main');
  r.git('rm', 'x_test.go');
  r.write('replacement.test.ts', 'assert.equal(actual(), expected());\n');
  r.commit('replace test on main');
  r.git('checkout', '-q', 'side');
  r.git('merge', '-q', '--no-ff', 'main', '-m', 'reconcile');
  assert.equal(check(side, 'HEAD', r.dir).length, 1, 'a merge without a reason still fails');
  r.git('commit', '-q', '--amend', '-m', 'reconcile\n\nTest-Change: the obsolete Go fixture was replaced by replacement.test.ts on main');
  assert.deepEqual(check(side, 'HEAD', r.dir), [], 'the merge itself changes the test relative to its first parent');
});

test('an unrelated merge trailer cannot excuse an earlier test removal', () => {
  const r = repo();
  r.write('x_test.go', goTest('\tt.Fatal("a")'));
  const base = r.commit('base');
  r.git('checkout', '-q', '-b', 'side');
  r.write('side.go', 'package x\n');
  r.commit('side');
  r.git('checkout', '-q', 'main');
  r.git('rm', 'x_test.go');
  r.write('main.go', 'package x\n');
  r.commit('drop test without explanation');
  r.git('merge', '-q', '--no-ff', 'side', '-m', 'unrelated merge\n\nTest-Change: this message must not cover an unchanged test path');
  const found = check(base, 'HEAD', r.dir);
  assert.equal(found.length, 1);
  assert.equal(found[0].file, 'x_test.go');
});

test('the hooks read CI state and pushes correctly', () => {
  const runs = [
    { headBranch: 'main', status: 'completed', conclusion: 'success' },
    { headBranch: 'queue/main', status: 'completed', conclusion: 'failure' },
    { headBranch: 'queue/main', status: 'completed', conclusion: 'success' },
    { headBranch: 'main', status: 'completed', conclusion: 'failure' },
    { headBranch: 'feature', status: 'completed', conclusion: 'failure' },
    { headBranch: 'queue/x', status: 'in_progress', conclusion: '' },
  ];
  assert.deepEqual(redBranches(runs).map(r => r.headBranch), ['queue/main'], 'only the latest run of each branch counts');
  assert.deepEqual(pushedRef('git push origin main:queue/main'), { ref: 'main', branch: 'queue/main' });
  assert.deepEqual(pushedRef('git push -q origin main'), { ref: 'main', branch: 'main' });
  assert.deepEqual(pushedRef('git push'), { ref: 'HEAD', branch: null });
  const abs = path.resolve('/work/repo');
  assert.equal(pushDir(`cd ${abs} && git add x && git push origin main:queue/main`, '/session'), abs);
  assert.equal(pushDir(`git -C ${abs} push origin main`, '/session'), abs);
  assert.equal(pushDir('git push', path.resolve('/session')), path.resolve('/session'));
  if (process.platform === 'win32') assert.equal(pushDir('cd /d/localdev/x && git push', 'C:/s'), 'D:/localdev/x');
  const quiet = spawnSync(process.execPath, [path.join(here, 'agent-hooks.mjs'), 'test-edit'], { input: '{"tool_input":{"file_path":"src/app.ts"}}', encoding: 'utf8' });
  assert.equal(quiet.stdout, '', 'a non-test file adds nothing');
  const self = path.join(here, 'ci.test.mjs');
  const noted = spawnSync(process.execPath, [path.join(here, 'agent-hooks.mjs'), 'test-edit'], { input: JSON.stringify({ tool_input: { file_path: self } }), encoding: 'utf8' });
  assert.match(JSON.parse(noted.stdout).hookSpecificOutput.additionalContext, /fix the source, not the test/);
});

test('a pushed commit counts as promoted once main contains it, even after later commits', () => {
  const origin = fs.mkdtempSync(path.join(os.tmpdir(), 'origin-'));
  execFileSync('git', ['init', '-q', '--bare', '-b', 'main', origin]);
  const r = repo();
  r.git('remote', 'add', 'origin', origin);
  r.write('a.txt', '1');
  const first = r.commit('first');
  r.write('a.txt', '2');
  const second = r.commit('second');
  r.git('checkout', '-q', '-b', 'side', first);
  r.write('b.txt', 'x');
  const elsewhere = r.commit('not on main');
  assert.equal(inMain(first, r.dir), false, 'nothing pushed yet');
  r.git('push', '-q', 'origin', `${first}:refs/heads/main`);
  assert.equal(inMain(first, r.dir), true, 'main is at it');
  r.git('push', '-q', 'origin', `${second}:refs/heads/main`);
  assert.equal(inMain(first, r.dir), true, 'main moved past it');
  assert.equal(inMain(elsewhere, r.dir), false, 'a commit main does not contain');
});
