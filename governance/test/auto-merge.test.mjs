import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import yaml from 'js-yaml';

test('baseline auto-merge uses a trusted base workflow and required head binding', () => {
  const file = fs.readFileSync(new URL('../../.github/workflows/auto-merge.yml', import.meta.url), 'utf8');
  const workflow = yaml.load(file);
  assert.deepEqual(workflow.on.pull_request_target.branches, ['main']);
  assert.deepEqual(workflow.on.pull_request_target.types, ['opened', 'reopened', 'synchronize', 'ready_for_review']);
  assert.deepEqual(workflow.permissions, { contents: 'write', 'pull-requests': 'write' });
  const job = workflow.jobs.enable;
  assert.match(job.if, /draft == false/);
  assert.match(job.if, /head\.repo\.full_name == github\.repository/);
  assert.match(job.if, /author_association/);
  assert.equal(job.steps.length, 1);
  assert.equal(job.steps[0].uses, undefined);
  assert.match(job.steps[0].run, /--auto --merge --match-head-commit "\$HEAD_SHA"/);
  assert.match(job.steps[0].run, /\[0-9a-f\]\{40\}/);
});
