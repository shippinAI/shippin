import assert from 'node:assert/strict';
import { test } from 'node:test';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { exportLibrary } from './export.mjs';
import { LIMITS, lintLibrary } from './lint.mjs';

const here = path.dirname(fileURLToPath(import.meta.url));
const libraryRoot = path.resolve(here, '../../.agents/library');
const lintCli = path.join(here, 'lint.mjs');
const exportCli = path.join(here, 'export.mjs');
const asOf = new Date('2026-11-19T00:00:00Z');

function daysBefore(iso, days) {
  const [year, month, day] = iso.split('-').map(Number);
  return new Date(Date.UTC(year, month - 1, day) - days * 86400000).toISOString().slice(0, 10);
}

function skillMarkdown(name, description = 'Use when testing the skill linter.') {
  return `---\nname: ${name}\ndescription: ${description}\n---\n\n# Test\n\nDo the thing.\n`;
}

function factsMarkdown(verified = '2026-11-19') {
  return `# Facts\n\n## Example\n\n- claim: An example claim.\n- source: the test\n- verified: ${verified}\n`;
}

function testsMarkdown() {
  return `# Trigger tests\n\n## Should trigger\n\n- "Do the thing."\n\n## Should not trigger\n\n- "Talk about the weather."\n`;
}

function fixture(t, files) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'skills-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const written = { 'README.md': '# Fixture\n', ...files };
  for (const [relative, content] of Object.entries(written)) {
    const full = path.join(root, relative);
    fs.mkdirSync(path.dirname(full), { recursive: true });
    fs.writeFileSync(full, content);
  }
  return root;
}

function validTree(t, overrides = {}) {
  const name = 'shippin-demo-sample';
  return fixture(t, {
    'demo/sample/SKILL.md': overrides.skill ?? skillMarkdown(name),
    'demo/sample/facts.md': overrides.facts ?? factsMarkdown(),
    'demo/sample/tests.md': overrides.tests ?? testsMarkdown(),
  });
}

test('the canonical library is clean as of the fact dates', () => {
  const result = lintLibrary(libraryRoot, { now: asOf });
  assert.deepEqual(result.errors, [], result.errors.join('\n'));
  assert.deepEqual(result.warnings, []);
  assert.deepEqual(result.skills.filter(skill => skill.flatName).map(skill => skill.flatName), [
    'shippin-infra-change-control',
    'shippin-meta-skill-authoring',
  ]);
  assert.equal(result.skills.some(skill => skill.template), true);
});

test('lint CLI exits 0 on the canonical library', () => {
  const result = spawnSync(process.execPath, [lintCli], { encoding: 'utf8' });
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /2 skills, 0 errors/);
});

test('a valid skill passes, including a description of exactly the maximum', t => {
  const description = `Use when ${'a'.repeat(LIMITS.descriptionMax - 'Use when '.length)}`;
  assert.equal(Array.from(description).length, LIMITS.descriptionMax);
  const root = validTree(t, { skill: skillMarkdown('shippin-demo-sample', `"${description}"`) });
  const result = lintLibrary(root, { now: asOf });
  assert.deepEqual(result.errors, [], result.errors.join('\n'));
});

test('frontmatter, trigger, size, name, and sibling failures are errors', t => {
  const cases = [
    ['demo/sample/SKILL.md', '# No frontmatter\n', /must start with ---/],
    ['demo/sample/SKILL.md', '---\nname: shippin-demo-sample\n', /not closed/],
    ['demo/sample/SKILL.md', skillMarkdown('shippin-demo-sample', ''), /description is empty/],
    ['demo/sample/SKILL.md', skillMarkdown('shippin-demo-sample', `Use when ${'a'.repeat(LIMITS.descriptionMax)}`), /maximum is 300/],
    ['demo/sample/SKILL.md', skillMarkdown('shippin-demo-sample', 'Helps with deploys.'), /must start with "Use when "/],
    ['demo/sample/SKILL.md', skillMarkdown('shippin-other', 'Use when testing the skill linter.'), /flat export name is shippin-demo-sample/],
    ['demo/sample/SKILL.md', `---\nname: shippin-demo-sample\ndescription: Use when one\n  continues\n---\n\n# Test\n\nBody.\n`, /not a simple key/],
    ['demo/sample/SKILL.md', `---\nname: shippin-demo-sample\ndescription: Use when testing.\nlicense: Apache-2.0\n---\n\n# Test\n\nBody.\n`, /unexpected frontmatter key license/],
    ['demo/sample/SKILL.md', `---\nname: shippin-demo-sample\ndescription: Use when testing.\n---\n`, /body is empty/],
    ['demo/sample/SKILL.md', `---\nname: shippin-demo-sample\ndescription: Use when testing.\n---\n\n${'line\n'.repeat(LIMITS.bodyMaxLines + 1)}`, /maximum is 120/],
    ['demo/sample/SKILL.md', `---\nname: shippin-demo-sample\ndescription: Use when testing.\n---\n\n${'x'.repeat(LIMITS.bodyMaxBytes + 1)}\n`, /maximum is 6000/],
  ];
  for (const [file, content, pattern] of cases) {
    const root = validTree(t, { skill: content });
    assert.equal(file, 'demo/sample/SKILL.md');
    const result = lintLibrary(root, { now: asOf });
    assert.ok(result.errors.some(error => pattern.test(error)), `${pattern}: ${result.errors.join(' | ')}`);
  }
  const missingFacts = fixture(t, {
    'demo/sample/SKILL.md': skillMarkdown('shippin-demo-sample'),
    'demo/sample/tests.md': testsMarkdown(),
  });
  assert.match(lintLibrary(missingFacts, { now: asOf }).errors.join('\n'), /missing facts.md/);
  const missingPrompt = validTree(t, { tests: '# Trigger tests\n\n## Should trigger\n\n- "Yes."\n\n## Should not trigger\n\nNone.\n' });
  assert.match(lintLibrary(missingPrompt, { now: asOf }).errors.join('\n'), /Should not trigger/);
});

test('verified dates fail when invalid or future, and warn after 90 days', t => {
  const future = validTree(t, { facts: factsMarkdown('2026-11-20') });
  assert.match(lintLibrary(future, { now: asOf }).errors.join('\n'), /in the future/);
  const impossible = validTree(t, { facts: factsMarkdown('2026-02-31') });
  assert.match(lintLibrary(impossible, { now: asOf }).errors.join('\n'), /not a real YYYY-MM-DD/);
  const undated = validTree(t, { facts: '# Facts\n\n## Example\n\n- claim: No date.\n' });
  assert.match(lintLibrary(undated, { now: asOf }).errors.join('\n'), /has no verified: date/);

  const fresh = validTree(t, { facts: factsMarkdown(daysBefore('2026-11-19', LIMITS.factMaxAgeDays)) });
  const freshResult = lintLibrary(fresh, { now: asOf });
  assert.deepEqual(freshResult.errors, [], freshResult.errors.join('\n'));
  assert.deepEqual(freshResult.warnings, []);

  const stale = validTree(t, { facts: factsMarkdown(daysBefore('2026-11-19', LIMITS.factMaxAgeDays + 1)) });
  const staleResult = lintLibrary(stale, { now: asOf });
  assert.deepEqual(staleResult.errors, [], staleResult.errors.join('\n'));
  assert.equal(staleResult.warnings.length, 1);
  assert.match(staleResult.warnings[0], /91 days old/);
});

test('secret-shaped text in any sibling file fails the lint', t => {
  // Pieces stay split in this file so a secret scanner does not treat the fixture source as a leak.
  const awsKey = ['AKIA', 'IOSFODNN7EXAMPLE'].join('');
  const privateKey = ['-----BEGIN OPENSSH ', 'PRIVATE KEY-----'].join('');
  const assigned = ['token: "', 'supersecretvalue', '"'].join('');
  const samples = [
    ['SKILL.md', `${skillMarkdown('shippin-demo-sample')}\n${awsKey}\n`, /aws-access-key/],
    ['facts.md', `${factsMarkdown()}\n${privateKey}\n`, /private-key/],
    ['tests.md', `${testsMarkdown()}\n${assigned}\n`, /assigned-secret/],
  ];
  for (const [name, content, pattern] of samples) {
    const root = validTree(t, { [name === 'SKILL.md' ? 'skill' : name === 'facts.md' ? 'facts' : 'tests']: content });
    assert.match(lintLibrary(root, { now: asOf }).errors.join('\n'), pattern);
  }
});

test('the tree rejects a skill buried in an area file, a nested folder, and a second template', t => {
  const areaSkill = fixture(t, { 'demo/SKILL.md': skillMarkdown('shippin-demo'), 'demo/facts.md': factsMarkdown(), 'demo/tests.md': testsMarkdown() });
  assert.match(lintLibrary(areaSkill, { now: asOf }).errors.join('\n'), /cannot contain SKILL.md/);
  const nested = validTree(t, {});
  fs.mkdirSync(path.join(nested, 'demo/sample/notes'));
  assert.match(lintLibrary(nested, { now: asOf }).errors.join('\n'), /subdirectory/);
  const extra = validTree(t, {});
  fs.mkdirSync(path.join(extra, '_draft'));
  assert.match(lintLibrary(extra, { now: asOf }).errors.join('\n'), /only _template/);
});

test('export flattens published skills and leaves the template behind', t => {
  const out = fs.mkdtempSync(path.join(os.tmpdir(), 'skills-out-'));
  t.after(() => fs.rmSync(out, { recursive: true, force: true }));
  fs.mkdirSync(path.join(out, 'shippin-old'));
  fs.writeFileSync(path.join(out, 'shippin-old/SKILL.md'), 'stale\n');
  const { written } = exportLibrary(libraryRoot, out);
  assert.deepEqual(written, ['shippin-infra-change-control', 'shippin-meta-skill-authoring']);
  for (const name of written) {
    const area = name === 'shippin-infra-change-control' ? 'infra/change-control' : 'meta/skill-authoring';
    assert.equal(
      fs.readFileSync(path.join(out, name, 'SKILL.md'), 'utf8'),
      fs.readFileSync(path.join(libraryRoot, area, 'SKILL.md'), 'utf8'),
    );
    assert.equal(fs.existsSync(path.join(out, name, 'facts.md')), false);
  }
  assert.equal(fs.existsSync(path.join(out, '_template')), false);
  assert.equal(fs.readFileSync(path.join(out, 'shippin-old/SKILL.md'), 'utf8'), 'stale\n');
});

test('export refuses the library directory and a library the linter rejects', t => {
  assert.throws(() => exportLibrary(libraryRoot, path.join(libraryRoot, 'out')), /inside the canonical library/);
  const broken = validTree(t, { skill: '# nope\n' });
  const out = fs.mkdtempSync(path.join(os.tmpdir(), 'skills-out-'));
  t.after(() => fs.rmSync(out, { recursive: true, force: true }));
  assert.throws(() => exportLibrary(broken, out), /must start with ---/);
  assert.equal(fs.readdirSync(out).length, 0);
});

test('export CLI writes the flat tree and does not require a network', t => {
  const out = fs.mkdtempSync(path.join(os.tmpdir(), 'skills-cli-'));
  t.after(() => fs.rmSync(out, { recursive: true, force: true }));
  const result = spawnSync(process.execPath, [exportCli, out], { encoding: 'utf8' });
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /shippin-infra-change-control\/SKILL.md/);
  assert.match(result.stdout, /2 skills exported/);
  const missing = spawnSync(process.execPath, [exportCli], { encoding: 'utf8' });
  assert.equal(missing.status, 2);
});
