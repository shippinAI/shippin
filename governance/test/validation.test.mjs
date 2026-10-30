import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import crypto from 'node:crypto';
import { validatePlan, validateJournal } from '../validate.mjs';

function sample(hash) {
  return { schema_version: '1', id: 'work-1', objective: 'Deliver', scope: 'fixture',
    context: { facts: [], assumptions: [], audience: 'operator', response: 'report' },
    sources: [{ path: 'source.txt', sha256: hash, observation: 'Read source' }],
    steps: [{ id: 'one', action: 'Change', expected: 'Works', failure_action: 'Stop' }],
    checks: [{ id: 'test', purpose: 'Behavior', command: 'test command', expected: 'Pass', evidence: 'log' }], unknowns: [] };
}
function entry(state, results) {
  return { schema_version: '1', plan_id: 'work-1', timestamp: '2026-10-29T00:00:00Z', actor: 'test', state, summary: 'Observed', results, next: 'Review' };
}

test('baseline detects source changes and paths escaping repository', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'shippin-contract-'));
  try {
    fs.writeFileSync(path.join(root, 'source.txt'), 'original');
    const hash = crypto.createHash('sha256').update('original').digest('hex');
    const plan = sample(hash);
    validatePlan(plan, { baseline: true, root });
    fs.writeFileSync(path.join(root, 'source.txt'), 'changed');
    assert.throws(() => validatePlan(plan, { baseline: true, root }), /stale source/);
    plan.sources[0].path = path.join(root, 'source.txt');
    assert.throws(() => validatePlan(plan, { baseline: true, root }), /relative/);
  } finally { fs.rmSync(root, { recursive: true, force: true }); }
});

test('completion requires actual named checks and evidence; failures stay visible', () => {
  const plan = sample('a'.repeat(64));
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'shippin-journal-'));
  try {
  fs.writeFileSync(path.join(root, 'log.txt'), 'exit 0');
  const evidence = { path: 'log.txt', sha256: crypto.createHash('sha256').update('exit 0').digest('hex') };
  assert.throws(() => validateJournal(plan, [entry('complete', [])]), /requires passing/);
  assert.throws(() => validateJournal(plan, [entry('complete', [{ check_id: 'test', status: 'passed', detail: 'ok' }])]), /lacks evidence/);
  assert.throws(() => validateJournal(plan, [entry('complete', [{ check_id: 'other', status: 'passed', detail: 'ok', evidence }])], { root }), /unknown check/);
  const pass = { check_id: 'test', status: 'passed', detail: 'exit 0', evidence };
  validateJournal(plan, [entry('in_progress', [{ ...pass, status: 'failed' }]), entry('complete', [pass])], { root });
  assert.throws(() => validateJournal(plan, [entry('complete', [pass]), entry('handoff', [])], { root }), /after completion/);
  fs.writeFileSync(path.join(root, 'log.txt'), 'changed');
  assert.throws(() => validateJournal(plan, [entry('complete', [pass])], { root }), /stale evidence/);
  } finally { fs.rmSync(root, { recursive: true, force: true }); }
});
