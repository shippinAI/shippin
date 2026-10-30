import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { fileURLToPath } from 'node:url';
import Ajv2020 from 'ajv/dist/2020.js';
import { validatePlan, validateJournal } from './validate.mjs';

const here = path.dirname(fileURLToPath(import.meta.url));
const defaultRoot = path.resolve(here, '..');
const ajv = new Ajv2020({ allErrors: true, strict: true });
const schema = name => ajv.compile(JSON.parse(fs.readFileSync(path.join(here, 'schemas', name), 'utf8')));
const checkSession = schema('session.v1.schema.json');
const checkChat = schema('chat-entry.v1.schema.json');
const sessionId = /^[0-9]{4}\/(?:0[1-9]|1[0-2])\/[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const planId = /^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$/;
const now = () => new Date().toISOString().replace(/\.\d{3}Z$/, 'Z');

function assertSchema(check, data, label) {
  if (!check(data)) throw new Error(`${label}: ${ajv.errorsText(check.errors)}`);
}
function safeSession(root, id) {
  if (!sessionId.test(id)) throw new Error('invalid session id');
  const base = fs.realpathSync(path.join(root, '.ai', 'data', 'sessions'));
  const dir = fs.realpathSync(path.join(base, id));
  const rel = path.relative(base, dir);
  if (!rel || rel === '..' || rel.startsWith(`..${path.sep}`) || path.isAbsolute(rel)) throw new Error('session outside data directory');
  const meta = JSON.parse(fs.readFileSync(path.join(dir, 'session.json'), 'utf8'));
  assertSchema(checkSession, meta, 'session');
  if (meta.id !== id) throw new Error('session id mismatch');
  return dir;
}
function withLock(dir, fn) {
  const lock = path.join(dir, '.write.lock');
  let fd;
  try { fd = fs.openSync(lock, 'wx', 0o600); }
  catch (error) { if (error.code === 'EEXIST') throw new Error('session is locked; inspect the writer before removing a stale lock'); throw error; }
  try { return fn(); } finally { fs.closeSync(fd); fs.unlinkSync(lock); }
}
function appendLine(file, data) {
  const fd = fs.openSync(file, 'a', 0o600);
  try { fs.writeFileSync(fd, `${JSON.stringify(data)}\n`, 'utf8'); fs.fsyncSync(fd); }
  finally { fs.closeSync(fd); }
}
function lines(file) {
  if (!fs.existsSync(file)) return [];
  const raw = fs.readFileSync(file, 'utf8');
  if (raw && !raw.endsWith('\n')) throw new Error(`incomplete record in ${path.basename(file)}`);
  return raw.split('\n').filter(Boolean).map(line => JSON.parse(line));
}
export function createSession(root, objective, captureScope) {
  const timestamp = now();
  const id = `${timestamp.slice(0, 4)}/${timestamp.slice(5, 7)}/${crypto.randomUUID()}`;
  const meta = { schema_version: '1', id, created_at: timestamp, objective, capture_scope: captureScope };
  assertSchema(checkSession, meta, 'session');
  const dir = path.join(root, '.ai', 'data', 'sessions', id);
  fs.mkdirSync(dir, { recursive: true, mode: 0o700 });
  fs.writeFileSync(path.join(dir, 'session.json'), `${JSON.stringify(meta, null, 2)}\n`, { flag: 'wx', mode: 0o600 });
  return meta;
}
export function addChat(root, id, role, representation, content) {
  const dir = safeSession(root, id);
  const entry = { schema_version: '1', id: crypto.randomUUID(), timestamp: now(), role, representation, content };
  assertSchema(checkChat, entry, 'chat');
  return withLock(dir, () => { lines(path.join(dir, 'chat.jsonl')); appendLine(path.join(dir, 'chat.jsonl'), entry); return entry; });
}
export function addPlan(root, id, plan) {
  const dir = safeSession(root, id);
  validatePlan(plan);
  if (!planId.test(plan.id)) throw new Error('plan id must use letters, numbers, underscore or hyphen');
  return withLock(dir, () => {
    fs.mkdirSync(path.join(dir, 'plans'), { recursive: true, mode: 0o700 });
    fs.writeFileSync(path.join(dir, 'plans', `${plan.id}.json`), `${JSON.stringify(plan, null, 2)}\n`, { flag: 'wx', mode: 0o600 });
  });
}
export function addJournal(root, id, entry) {
  const dir = safeSession(root, id);
  if (entry.schema_version !== '2') throw new Error('new sessions require journal schema version 2');
  if (!planId.test(entry.plan_id ?? '')) throw new Error('invalid plan id');
  return withLock(dir, () => {
    const plan = JSON.parse(fs.readFileSync(path.join(dir, 'plans', `${entry.plan_id}.json`), 'utf8'));
    const folder = path.join(dir, 'journal');
    fs.mkdirSync(folder, { recursive: true, mode: 0o700 });
    const file = path.join(folder, `${entry.plan_id}.jsonl`);
    validateJournal(plan, [...lines(file), entry], { root });
    appendLine(file, entry);
  });
}
export function resume(root, id) {
  const dir = safeSession(root, id);
  const meta = JSON.parse(fs.readFileSync(path.join(dir, 'session.json'), 'utf8'));
  const chat = lines(path.join(dir, 'chat.jsonl'));
  for (const item of chat) assertSchema(checkChat, item, 'chat');
  const planDir = path.join(dir, 'plans');
  const plans = fs.existsSync(planDir) ? fs.readdirSync(planDir).filter(name => name.endsWith('.json')).sort().map(name => {
    const plan = JSON.parse(fs.readFileSync(path.join(planDir, name), 'utf8'));
    validatePlan(plan);
    if (!planId.test(plan.id) || name !== `${plan.id}.json`) throw new Error('plan filename mismatch');
    const journal = lines(path.join(dir, 'journal', `${plan.id}.jsonl`));
    if (journal.length) validateJournal(plan, journal, { root });
    return { plan, journal };
  }) : [];
  return { session: meta, chat, plans };
}
export function listSessions(root) {
  const base = path.join(root, '.ai', 'data', 'sessions');
  if (!fs.existsSync(base)) return [];
  const found = [];
  for (const year of fs.readdirSync(base).filter(x => /^\d{4}$/.test(x)).sort()) {
    for (const month of fs.readdirSync(path.join(base, year)).filter(x => /^(0[1-9]|1[0-2])$/.test(x)).sort()) {
      for (const uuid of fs.readdirSync(path.join(base, year, month)).sort()) {
        const id = `${year}/${month}/${uuid}`;
        if (sessionId.test(id)) {
          const dir = safeSession(root, id);
          found.push(JSON.parse(fs.readFileSync(path.join(dir, 'session.json'), 'utf8')));
        }
      }
    }
  }
  return found;
}
function main(args) {
  const [command, id, a, b] = args;
  if (command === 'create' && id && a && !b) return createSession(defaultRoot, id, a);
  if (command === 'chat' && id && a && b) return addChat(defaultRoot, id, a, b, fs.readFileSync(0, 'utf8'));
  if (command === 'plan' && id && !a) return addPlan(defaultRoot, id, JSON.parse(fs.readFileSync(0, 'utf8'))) ?? { ok: true };
  if (command === 'journal' && id && !a) return addJournal(defaultRoot, id, JSON.parse(fs.readFileSync(0, 'utf8'))) ?? { ok: true };
  if (command === 'resume' && id && !a) return resume(defaultRoot, id);
  if (command === 'list' && !id) return listSessions(defaultRoot);
  throw new Error('usage: session.mjs create OBJECTIVE CAPTURE_SCOPE | chat ID ROLE verbatim|summary < text | plan ID < plan.json | journal ID < entry.json | resume ID | list');
}
if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try { console.log(JSON.stringify(main(process.argv.slice(2)), null, 2)); }
  catch (error) { console.error(error.message); process.exitCode = 1; }
}
