import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { fileURLToPath } from 'node:url';
import Ajv2020 from 'ajv/dist/2020.js';

const here = path.dirname(fileURLToPath(import.meta.url));
const repository = path.resolve(here, '..');
const load = file => JSON.parse(fs.readFileSync(file, 'utf8'));
const ajv = new Ajv2020({ allErrors: true, strict: true });
const planSchema = ajv.compile(load(path.join(here, 'schemas/plan.v1.schema.json')));
const journalSchemas = new Map(['1', '2'].map(version => [version,
  ajv.compile(load(path.join(here, `schemas/journal-entry.v${version}.schema.json`)))]));

function checkSchema(check, object, label) {
  if (!check(object)) throw new Error(`${label}: ${ajv.errorsText(check.errors)}`);
}

function uniqueIds(items, label) {
  const seen = new Set();
  for (const item of items) {
    if (seen.has(item.id)) throw new Error(`duplicate ${label} id: ${item.id}`);
    seen.add(item.id);
  }
}

function verifyFile(root, name, expected, label) {
  const base = fs.realpathSync(root);
  if (path.isAbsolute(name)) throw new Error(`${label} path must be relative: ${name}`);
  const file = fs.realpathSync(path.resolve(base, name));
  const relative = path.relative(base, file);
  if (!relative || relative === '..' || relative.startsWith(`..${path.sep}`) || path.isAbsolute(relative)) {
    throw new Error(`${label} outside repository: ${name}`);
  }
  const digest = crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
  if (digest !== expected) throw new Error(`stale ${label}: ${name}`);
}

export function validatePlan(plan, { baseline = false, root = repository } = {}) {
  checkSchema(planSchema, plan, 'plan');
  uniqueIds(plan.steps, 'step');
  uniqueIds(plan.checks, 'check');
  if (!baseline) return;
  for (const source of plan.sources) {
    verifyFile(root, source.path, source.sha256, 'source');
  }
}

export function validateJournal(plan, entries, { root = repository } = {}) {
  validatePlan(plan);
  if (!entries.length) throw new Error('journal is empty');
  const checks = new Set(plan.checks.map(check => check.id));
  const steps = new Set(plan.steps.map(step => step.id));
  const latest = new Map();
  const latestSteps = new Map();
  const version = entries[0].schema_version;
  let complete = false;
  for (const [index, entry] of entries.entries()) {
    const validator = journalSchemas.get(entry.schema_version);
    if (!validator || entry.schema_version !== version) throw new Error(`journal entry ${index + 1}: unknown or mixed schema version`);
    checkSchema(validator, entry, `journal entry ${index + 1}`);
    if (entry.plan_id !== plan.id) throw new Error(`journal entry ${index + 1}: wrong plan_id`);
    if (complete) throw new Error('entries after completion require a new or revised plan');
    for (const result of entry.step_results ?? []) {
      if (!steps.has(result.step_id)) throw new Error(`unknown step: ${result.step_id}`);
      if (result.status === 'done' && !result.evidence) throw new Error(`done step lacks evidence: ${result.step_id}`);
      if (result.evidence) verifyFile(root, result.evidence.path, result.evidence.sha256, 'evidence');
      if (result.status === 'done') {
        for (const prior of plan.steps) {
          if (prior.id === result.step_id) break;
          if (latestSteps.get(prior.id) !== 'done') throw new Error(`step ${result.step_id} requires preceding step ${prior.id}`);
        }
      }
      latestSteps.set(result.step_id, result.status);
    }
    for (const result of entry.results) {
      if (!checks.has(result.check_id)) throw new Error(`unknown check: ${result.check_id}`);
      if (result.status === 'passed' && !result.evidence) throw new Error(`passed check lacks evidence: ${result.check_id}`);
      if (result.evidence) verifyFile(root, result.evidence.path, result.evidence.sha256, 'evidence');
      latest.set(result.check_id, result.status);
    }
    if (entry.state === 'complete') {
      if (version === '2') for (const step of steps) {
        if (latestSteps.get(step) !== 'done') throw new Error(`completion requires done step: ${step}`);
      }
      for (const check of checks) {
        if (latest.get(check) !== 'passed') throw new Error(`completion requires passing check: ${check}`);
      }
      complete = true;
    }
  }
}

function main(args) {
  const [mode, planFile, extra] = args;
  if (!planFile || !['plan', 'baseline', 'journal'].includes(mode) || (mode === 'journal' ? !extra : extra)) {
    throw new Error('usage: node governance/validate.mjs plan|baseline PLAN.json; or journal PLAN.json JOURNAL.jsonl');
  }
  const plan = load(planFile);
  if (mode === 'journal') {
    const lines = fs.readFileSync(extra, 'utf8').split(/\r?\n/).filter(line => line.trim());
    validateJournal(plan, lines.map(line => JSON.parse(line)));
  } else validatePlan(plan, { baseline: mode === 'baseline' });
  console.log(`${mode} valid`);
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try { main(process.argv.slice(2)); } catch (err) { console.error(err.message); process.exitCode = 1; }
}
