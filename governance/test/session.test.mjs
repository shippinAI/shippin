import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import crypto from 'node:crypto';
import { createSession, addChat, addPlan, addJournal, listSessions, resume } from '../session.mjs';

const sha = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
function sample() {
  return { schema_version: '1', id: 'first', objective: 'Ship', scope: 'fixture',
    context: { facts: [], assumptions: [], audience: 'operator', response: 'report' },
    sources: [{ path: 'evidence.txt', sha256: sha('verified'), observation: 'Observed' }],
    steps: [{ id: 'change', action: 'Change', expected: 'Result', failure_action: 'Stop' }],
    checks: [{ id: 'outcome', purpose: 'Verify outcome', command: 'run check', expected: 'Pass', evidence: 'evidence.txt' }], unknowns: [] };
}
function entry(state, step_results, results) {
  return { schema_version: '2', plan_id: 'first', timestamp: '2026-10-29T00:00:00Z', actor: 'test',
    state, summary: 'Observed', step_results, results, next: 'Continue' };
}

test('session records chat, plan and verified steps, then resumes across calls', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'shippin-session-'));
  try {
    const session = createSession(root, 'Ship objective', 'Only supplied messages are captured');
    assert.equal(listSessions(root)[0].id, session.id);
    addChat(root, session.id, 'user', 'verbatim', 'first line\nsecond line');
    addChat(root, session.id, 'assistant', 'summary', 'Working on it');
    assert.throws(() => addChat(root, session.id, 'tool', 'verbatim', 'bad'), /chat/);
    addPlan(root, session.id, sample());
    assert.throws(() => addPlan(root, session.id, sample()), /EEXIST/);
    fs.writeFileSync(path.join(root, 'evidence.txt'), 'verified');
    const evidence = { path: 'evidence.txt', sha256: sha('verified') };
    addJournal(root, session.id, entry('started', [], []));
    assert.throws(() => addJournal(root, session.id, entry('complete', [], [])), /done step/);
    assert.throws(() => addJournal(root, session.id, { ...entry('started', [], []), schema_version: '1' }), /version 2/);
    addJournal(root, session.id, entry('complete', [{ step_id: 'change', status: 'done', detail: 'Changed', evidence }],
      [{ check_id: 'outcome', status: 'passed', detail: 'exit 0', evidence }]));
    const result = resume(root, session.id);
    assert.equal(result.chat[0].content, 'first line\nsecond line');
    assert.equal(result.plans[0].journal.at(-1).state, 'complete');
    assert.throws(() => addJournal(root, session.id, entry('handoff', [], [])), /after completion/);
    fs.writeFileSync(path.join(root, 'evidence.txt'), 'tampered');
    assert.throws(() => resume(root, session.id), /stale evidence/);
    assert.throws(() => resume(root, '../escape'), /invalid session id/);
  } finally { fs.rmSync(root, { recursive: true, force: true }); }
});

test('partial writes and concurrent writers fail closed', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'shippin-session-'));
  try {
    const session = createSession(root, 'Objective', 'manual capture');
    const dir = path.join(root, '.ai', 'data', 'sessions', session.id);
    fs.writeFileSync(path.join(dir, '.write.lock'), 'busy');
    assert.throws(() => addChat(root, session.id, 'user', 'verbatim', 'message'), /locked/);
    fs.unlinkSync(path.join(dir, '.write.lock'));
    fs.writeFileSync(path.join(dir, 'chat.jsonl'), '{');
    assert.throws(() => addChat(root, session.id, 'user', 'verbatim', 'message'), /incomplete record/);
    assert.throws(() => resume(root, session.id), /incomplete record/);
  } finally { fs.rmSync(root, { recursive: true, force: true }); }
});

test('journal completion follows plan step order', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'shippin-order-'));
  try {
    const session = createSession(root, 'Objective', 'manual capture');
    const plan = sample();
    plan.steps.push({ id: 'later', action: 'Continue', expected: 'Outcome', failure_action: 'Stop' });
    addPlan(root, session.id, plan);
    fs.writeFileSync(path.join(root, 'evidence.txt'), 'verified');
    const evidence = { path: 'evidence.txt', sha256: sha('verified') };
    assert.throws(() => addJournal(root, session.id, entry('in_progress', [{ step_id: 'later', status: 'done', detail: 'Done', evidence }], [])), /preceding step/);
    addJournal(root, session.id, entry('in_progress', [{ step_id: 'change', status: 'done', detail: 'Done', evidence },
      { step_id: 'later', status: 'done', detail: 'Done', evidence }], []));
  } finally { fs.rmSync(root, { recursive: true, force: true }); }
});
