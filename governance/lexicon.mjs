#!/usr/bin/env node
// Lexicon: one developer's own terms, kept so a bad habit is bridged once and cited after that.
//
// Two stores, never in a repository:
//   global   $AI_LEXICON_HOME or ~/.ai/lexicon   the glossary for every project, the chat corpus and
//                                                 the evidence for each entry
//   project  <project>/.ai/lexicon                terms only this project uses, and a copy of this
//                                                 project's chats and sightings (`sync`); refused
//                                                 unless Git ignores the folder
//
// Entries are proposed until the person confirms them. The importer only proposes; the hook only cites
// confirmed entries. Uses node built-ins only, so the hook runs without `npm ci`.
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import readline from 'node:readline';
import { execFileSync } from 'node:child_process';
import { parseArgs } from 'node:util';
import { fileURLToPath } from 'node:url';

export const KINDS = ['misspelling', 'wrong', 'vague', 'overloaded', 'synonym'];
export const STATUSES = ['proposed', 'confirmed', 'retired'];
const SCOPES = ['global', 'project'];
const ID = /^[a-z0-9][a-z0-9-]{0,63}$/;
const now = () => new Date().toISOString().replace(/\.\d{3}Z$/, 'Z');

export function globalHome(env = process.env) {
  return env.AI_LEXICON_HOME ? path.resolve(env.AI_LEXICON_HOME) : path.join(os.homedir(), '.ai', 'lexicon');
}
export function projectRoot(env = process.env, cwd = process.cwd()) {
  if (env.LEXICON_PROJECT) return path.resolve(env.LEXICON_PROJECT);
  if (env.CLAUDE_PROJECT_DIR) return path.resolve(env.CLAUDE_PROJECT_DIR);
  try {
    return execFileSync('git', ['rev-parse', '--show-toplevel'], { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
  } catch { return cwd; }
}
export const slug = s => s.toLowerCase().normalize('NFKD').replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 64);

function readJSON(file, fallback) {
  try { return JSON.parse(fs.readFileSync(file, 'utf8')); }
  catch (error) { if (error.code === 'ENOENT') return fallback; throw new Error(`${file}: ${error.message}`); }
}
function writeJSON(file, data) {
  fs.mkdirSync(path.dirname(file), { recursive: true, mode: 0o700 });
  const tmp = `${file}.${process.pid}.tmp`;
  fs.writeFileSync(tmp, `${JSON.stringify(data, null, 2)}\n`, { mode: 0o600 });
  fs.renameSync(tmp, file);
}
function writeLines(file, rows) {
  fs.mkdirSync(path.dirname(file), { recursive: true, mode: 0o700 });
  const tmp = `${file}.${process.pid}.tmp`;
  fs.writeFileSync(tmp, rows.map(r => JSON.stringify(r)).join('\n') + (rows.length ? '\n' : ''), { mode: 0o600 });
  fs.renameSync(tmp, file);
}
function readLines(file) {
  if (!fs.existsSync(file)) return [];
  return fs.readFileSync(file, 'utf8').split('\n').filter(Boolean).map(l => JSON.parse(l));
}

// ---- entries ----

export function checkEntry(e) {
  const fail = m => { throw new Error(`entry ${e?.id ?? '?'}: ${m}`); };
  if (!e || typeof e !== 'object') fail('not an object');
  if (!ID.test(e.id ?? '')) fail('id must be lower-case letters, digits and dashes');
  if (typeof e.term !== 'string' || !e.term.trim() || e.term.length > 80) fail('term is 1 to 80 characters');
  if (typeof e.meaning !== 'string' || !e.meaning.trim() || e.meaning.length > 400) fail('meaning is 1 to 400 characters');
  if (!Array.isArray(e.variants) || !e.variants.length) fail('at least one variant');
  for (const v of e.variants) {
    if (typeof v.text !== 'string' || !v.text.trim() || v.text.length > 80) fail('a variant is 1 to 80 characters');
    if (!KINDS.includes(v.kind)) fail(`variant kind is one of ${KINDS.join(', ')}`);
    if (v.ambiguous !== undefined && typeof v.ambiguous !== 'boolean') fail('ambiguous is true or false');
  }
  if (!Array.isArray(e.cues ?? []) || (e.cues ?? []).some(c => typeof c !== 'string' || !c.trim())) fail('cues are words');
  if (e.variants.some(v => v.ambiguous) && !(e.cues ?? []).length) fail('an ambiguous variant needs cues');
  if (!STATUSES.includes(e.status)) fail(`status is one of ${STATUSES.join(', ')}`);
  if (e.why !== undefined && (typeof e.why !== 'string' || e.why.length > 400)) fail('why is at most 400 characters');
  for (const k of ['created_at', 'updated_at']) if (typeof e[k] !== 'string') fail(`${k} is required`);
  return e;
}
function checkStore(d, label) {
  if (d?.schema_version !== 1 || !Array.isArray(d.entries)) throw new Error(`${label}: not a lexicon file`);
  const ids = new Set(), variants = new Map();
  for (const e of d.entries) {
    checkEntry(e);
    if (ids.has(e.id)) throw new Error(`${label}: two entries named ${e.id}`);
    ids.add(e.id);
    for (const v of e.variants) {
      const k = v.text.toLowerCase();
      if (variants.has(k) && variants.get(k) !== e.id) throw new Error(`${label}: "${v.text}" is a variant of ${variants.get(k)} and ${e.id}`);
      variants.set(k, e.id);
    }
  }
  return d;
}

// Git must ignore the project folder before anything personal is written there.
export function projectIgnored(root) {
  try {
    execFileSync('git', ['check-ignore', '-q', '.ai/lexicon/lexicon.json'], { cwd: root, stdio: 'ignore' });
    return true;
  } catch (error) {
    if (error.status === 1) return false;
    return !fs.existsSync(path.join(root, '.git')); // not a repository: nothing to leak into
  }
}

export class Lexicon {
  constructor({ home = globalHome(), root = projectRoot() } = {}) {
    this.home = home;
    this.root = root;
    this.dirs = { global: home, project: path.join(root, '.ai', 'lexicon') };
  }
  file(scope) { return path.join(this.dirs[scope], 'lexicon.json'); }
  load(scope) { return checkStore(readJSON(this.file(scope), { schema_version: 1, entries: [] }), this.file(scope)); }
  save(scope, d) {
    if (scope === 'project' && !projectIgnored(this.root)) throw new Error(`Git does not ignore ${this.dirs.project}: add /.ai/lexicon/ to .gitignore first`);
    writeJSON(this.file(scope), checkStore(d, this.file(scope)));
  }
  // Project entries come first, so a project's meaning wins inside that project.
  entries() {
    return [...this.load('project').entries.map(e => ({ ...e, scope: 'project' })), ...this.load('global').entries.map(e => ({ ...e, scope: 'global' }))];
  }
  find(id) {
    const e = this.entries().find(x => x.id === id);
    if (!e) throw new Error(`no entry ${id}`);
    return e;
  }
  add({ id, term, meaning, variants, cues = [], why, scope = 'global' }) {
    if (!SCOPES.includes(scope)) throw new Error('scope is global or project');
    id = id ?? slug(term ?? '');
    if (this.entries().some(e => e.id === id)) throw new Error(`${id} exists`);
    const at = now();
    const entry = checkEntry({ id, term, meaning, variants, cues, ...(why ? { why } : {}), status: 'proposed', created_at: at, updated_at: at });
    const d = this.load(scope);
    d.entries.push(entry);
    this.save(scope, d);
    return entry;
  }
  update(id, fn) {
    const { scope } = this.find(id);
    const d = this.load(scope);
    const i = d.entries.findIndex(e => e.id === id);
    d.entries[i] = checkEntry({ ...fn(d.entries[i]), updated_at: now() });
    this.save(scope, d);
    return d.entries[i];
  }
  setStatus(id, status) {
    if (!STATUSES.includes(status)) throw new Error('unknown status');
    return this.update(id, e => ({ ...e, status, ...(status === 'confirmed' ? { confirmed_at: now() } : {}) }));
  }
  move(id, scope) {
    const e = this.find(id);
    if (e.scope === scope) return e;
    const from = this.load(e.scope), to = this.load(scope);
    const { scope: _, ...entry } = e;
    to.entries.push({ ...entry, updated_at: now() });
    this.save(scope, to);
    from.entries = from.entries.filter(x => x.id !== id);
    this.save(e.scope, from);
    return { ...entry, scope };
  }
  evidence() { return readJSON(path.join(this.home, 'evidence.json'), {}); }
}

// ---- text ----

const SECRET_VALUE = [
  /-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----/g,
  /\b(?:bearer|basic)\s+[A-Za-z0-9._~+/=-]{12,}/gi,
  /\b(?:sk|pk|rk)-[A-Za-z0-9_-]{16,}/g,
  /\bgh[pousr]_[A-Za-z0-9]{16,}/g,
  /\bxox[abprs]-[A-Za-z0-9-]{10,}/g,
  /\bAKIA[0-9A-Z]{16}\b/g,
  /\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{5,}/g,
  /\b(password|passwd|pwd|secret|token|api[_-]?key|client[_-]?secret)\s*[:=]\s*\S+/gi,
  /\b[A-Fa-f0-9]{32,}\b/g,
  /\b[A-Za-z0-9+/_-]{40,}={0,2}/g,
];
export function redact(s) {
  let out = s;
  for (const re of SECRET_VALUE) out = out.replace(re, m => (/^(password|passwd|pwd|secret|token|api|client)/i.test(m) ? `${m.split(/[:=]/)[0]}=[REDACTED]` : '[REDACTED]'));
  return out;
}
const WRAPPERS = /<(system-reminder|ide_selection|ide_opened_file|ide_diagnostics|command-message|command-name|command-args|local-command-stdout|local-command-stderr|local-command-caveat|task-notification|user-prompt-submit-hook)\b[^>]*>[\s\S]*?<\/\1>/g;
const PASTED = /<pasted_content\b[^>]*>[\s\S]*?<\/pasted_content\b[^>]*>/g;
// The person's own words in a message: no wrappers the tools add, no pasted material.
export function ownWords(raw) {
  const pasted = PASTED.test(raw);
  PASTED.lastIndex = 0;
  const text = raw.replace(PASTED, ' ').replace(WRAPPERS, ' ').replace(/[ \t]+\n/g, '\n').trim();
  return { text, pasted };
}
// Prose only: no code, links, paths or identifiers, for counting words.
export function prose(s) {
  return s.replace(/```[\s\S]*?```/g, ' ').replace(/`[^`\n]*`/g, ' ').replace(/\bhttps?:\/\/\S+/g, ' ')
    .replace(/\S*[\\/]\S*/g, ' ').replace(/\S+@\S+/g, ' ').replace(/\S*[_{}<>=$#|]\S*/g, ' ');
}
const WORD = /[a-z][a-z']*[a-z]|[a-z]/g;
export const words = s => (prose(s).toLowerCase().replace(/[‘’]/g, "'").match(WORD) ?? []);
const quote = (s, at, len) => redact(s.slice(Math.max(0, at - 80), at + len + 80).replace(/\s+/g, ' ').trim());
const escape = s => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

// Where each variant of each entry appears; an ambiguous variant counts only near one of its cues.
export function matches(text, entries) {
  const t = text.toLowerCase().replace(/[‘’]/g, "'");
  const hits = [];
  for (const e of entries) {
    const cues = (e.cues ?? []).map(c => new RegExp(`(?<![a-z0-9])${escape(c.toLowerCase())}`));
    for (const v of e.variants) {
      if (v.text.toLowerCase() === e.term.toLowerCase()) continue;
      const re = new RegExp(`(?<![a-z0-9])${escape(v.text.toLowerCase()).replace(/(?:\\-|\s)+/g, '[\\s-]+')}(?![a-z0-9])`, 'g');
      for (const m of t.matchAll(re)) {
        if (v.ambiguous) {
          const near = t.slice(Math.max(0, m.index - 60), m.index + m[0].length + 60); // about ten words each side
          if (!cues.some(c => c.test(near))) continue;
        }
        hits.push({ entry: e, variant: v, at: m.index, end: m.index + m[0].length, quote: quote(text, m.index, m[0].length) });
      }
    }
  }
  // A hit inside a longer one ("tenet" inside "multi-tenet") is the same word.
  // Two variants on one span ("multi-tenet", "multi tenet") count once.
  return hits.filter((h, i) => !hits.some((o, j) => j !== i && o.at <= h.at && o.end >= h.end && (o.end - o.at > h.end - h.at || (o.at === h.at && o.end === h.end && j < i))))
    .sort((a, b) => a.at - b.at);
}

// ---- the prompt hook ----

export function hookOutput(lex, input) {
  const prompt = String(input?.prompt ?? '');
  if (!prompt) return '';
  const { text } = ownWords(prompt);
  const hits = matches(prose(text), lex.entries().filter(e => e.status === 'confirmed'));
  if (!hits.length) return '';
  const evidence = lex.evidence();
  const seen = new Set();
  const lines = [];
  const at = now();
  const sightings = [];
  for (const h of hits) {
    sightings.push({ at, entry: h.entry.id, variant: h.variant.text, project: lex.root, session: input.session_id ?? null });
    if (seen.has(h.entry.id)) continue;
    seen.add(h.entry.id);
    const first = evidence[h.entry.id]?.first;
    lines.push(`- "${h.variant.text}" means ${h.entry.term} (${h.variant.kind}): ${h.entry.meaning}${first ? ` First seen ${first.slice(0, 10)}.` : ''}`);
  }
  fs.mkdirSync(lex.home, { recursive: true, mode: 0o700 });
  fs.appendFileSync(path.join(lex.home, 'sightings.jsonl'), sightings.map(s => JSON.stringify(s)).join('\n') + '\n', { mode: 0o600 });
  const context = 'Lexicon: terms this person has confirmed. Read their words with these meanings, use the preferred term, and name each correction once, briefly, without lecturing.\n' + lines.join('\n');
  return JSON.stringify({ hookSpecificOutput: { hookEventName: 'UserPromptSubmit', additionalContext: context } });
}

// ---- importing chats ----

const CORRECTION = /^\s*(?:no\b|nope\b|not\s+(?:that|what|quite)|wrong\b|stop\b|wait\b|that'?s\s+not\b|thats\s+not\b|i\s+said\b|i\s+asked\b|why\s+(?:did|are|is|would)\s+you\b|you\s+(?:didn'?t|did\s+not|missed|forgot|broke)\b|again\b|still\s+(?:not|broken|wrong|failing)\b|undo\b|revert\b)/i;
export const VAGUE = ['properly', 'best practices', 'best practice', 'smart', 'optimized', 'seamless', 'seamlessly', 'truly', 'true', 'really', 'actually', 'proper', 'stuff', 'things', 'etc', 'i guess', 'maybe', 'somehow', 'clean', 'robust', 'correctly'];

async function* jsonl(file) {
  const rl = readline.createInterface({ input: fs.createReadStream(file, { encoding: 'utf8' }), crlfDelay: Infinity });
  for await (const line of rl) yield line;
}
function* walk(dir, test) {
  let list;
  try { list = fs.readdirSync(dir, { withFileTypes: true }); } catch { return; }
  for (const d of list) {
    const p = path.join(dir, d.name);
    if (d.isDirectory()) { if (d.name !== 'subagents' && d.name !== 'memory') yield* walk(p, test); }
    else if (test(d.name)) yield p;
  }
}
const blockText = c => (typeof c === 'string' ? c : Array.isArray(c) ? c.filter(b => b?.type === 'text' || b?.type === 'output_text' || b?.type === 'input_text').map(b => b.text ?? '').join('\n') : '');

// Claude Code: ~/.claude/projects/<project>/<session>.jsonl. Automated sessions (sdk-*) and sub-agents are skipped.
async function claudeCode(dir, sink) {
  for (const file of walk(dir, n => n.endsWith('.jsonl'))) {
    const session = path.basename(file, '.jsonl');
    for await (const line of jsonl(file)) {
      const user = line.includes('"type":"user"'), assistant = line.includes('"type":"assistant"');
      if (!user && !assistant) continue;
      if (user && line.includes('"tool_result"')) continue;
      let r;
      try { r = JSON.parse(line); } catch { continue; }
      if (r.isSidechain || /^sdk/i.test(r.entrypoint ?? '')) continue;
      if (r.type === 'assistant') { sink.assistant(blockText(r.message?.content)); continue; }
      if (r.type !== 'user' || r.isMeta || r.isCompactSummary) continue;
      const raw = blockText(r.message?.content);
      if (/^\[Request interrupted/.test(raw.trim())) { sink.interrupt('claude-code', session); continue; }
      if (/^This session is being continued from a previous conversation/.test(raw) || /^Caveat:/.test(raw)) continue;
      sink.user({ source: 'claude-code', session, at: r.timestamp, cwd: r.cwd ?? null, raw });
    }
  }
}
// Codex: history.jsonl holds every prompt; the rollouts hold the replies and each session's folder.
async function codex(dir, sink) {
  const cwd = new Map();
  for (const file of walk(dir, n => /^rollout-.*\.jsonl$/.test(n))) {
    let first = true, id = null;
    for await (const line of jsonl(file)) {
      if (first) {
        first = false;
        try { const m = JSON.parse(line).payload; id = m?.id ?? m?.session_id; if (id) cwd.set(id, m.cwd ?? null); } catch { /* not a rollout */ }
        continue;
      }
      if (!line.includes('"role":"assistant"')) continue;
      try { const r = JSON.parse(line); if (r.payload?.type === 'message') sink.assistant(blockText(r.payload.content)); } catch { /* skip */ }
    }
  }
  const history = path.join(dir, 'history.jsonl');
  if (!fs.existsSync(history)) return;
  for await (const line of jsonl(history)) {
    let r;
    try { r = JSON.parse(line); } catch { continue; }
    if (typeof r.text !== 'string') continue;
    sink.user({ source: 'codex', session: r.session_id, at: new Date(r.ts * 1000).toISOString(), cwd: cwd.get(r.session_id) ?? null, raw: r.text });
  }
}
const conversations = p => {
  const file = fs.statSync(p).isDirectory() ? path.join(p, 'conversations.json') : p;
  if (/\.zip$/i.test(file)) throw new Error(`${file}: unzip the export first and pass its folder`);
  return JSON.parse(fs.readFileSync(file, 'utf8'));
};
// claude.ai: Settings > Privacy > Export data, then conversations.json.
function claudeExport(p, sink) {
  for (const c of conversations(p)) {
    const msgs = [...(c.chat_messages ?? [])].sort((a, b) => String(a.created_at).localeCompare(String(b.created_at)));
    for (const m of msgs) {
      const text = m.text || blockText(m.content);
      if (m.sender === 'assistant') sink.assistant(text);
      else if (m.sender === 'human') sink.user({ source: 'claude-ai', session: c.uuid, title: c.name ?? null, at: m.created_at, cwd: null, raw: text });
    }
  }
}
// ChatGPT: Settings > Data controls > Export, then conversations.json.
function chatgptExport(p, sink) {
  for (const c of conversations(p)) {
    const msgs = Object.values(c.mapping ?? {}).map(n => n.message).filter(m => m?.content?.parts).sort((a, b) => (a.create_time ?? 0) - (b.create_time ?? 0));
    for (const m of msgs) {
      const text = m.content.parts.filter(x => typeof x === 'string').join('\n');
      if (m.author?.role === 'assistant') sink.assistant(text);
      else if (m.author?.role === 'user') sink.user({ source: 'chatgpt', session: c.id ?? c.conversation_id ?? c.title, title: c.title ?? null, at: new Date((m.create_time ?? c.create_time ?? 0) * 1000).toISOString(), cwd: null, raw: text });
    }
  }
}

// Words within one or two edits (SymSpell deletes), for spotting misspellings against the replies' words.
function deletes(w, depth) {
  const out = new Set([w]);
  let level = [w];
  for (let d = 0; d < depth; d++) {
    const next = [];
    for (const x of level) for (let i = 0; i < x.length; i++) { const y = x.slice(0, i) + x.slice(i + 1); if (!out.has(y)) { out.add(y); next.push(y); } }
    level = next;
  }
  return out;
}
export function distance(a, b) {
  const d = Array.from({ length: a.length + 1 }, (_, i) => [i, ...Array(b.length).fill(0)]);
  for (let j = 1; j <= b.length; j++) d[0][j] = j;
  for (let i = 1; i <= a.length; i++) for (let j = 1; j <= b.length; j++) {
    const cost = a[i - 1] === b[j - 1] ? 0 : 1;
    d[i][j] = Math.min(d[i - 1][j] + 1, d[i][j - 1] + 1, d[i - 1][j - 1] + cost);
    if (i > 1 && j > 1 && a[i - 1] === b[j - 2] && a[i - 2] === b[j - 1]) d[i][j] = Math.min(d[i][j], d[i - 2][j - 2] + 1);
  }
  return d[a.length][b.length];
}
export function misspellings(userCounts, vocab, { minVocab = 5 } = {}) {
  const index = new Map();
  for (const [w, n] of vocab) {
    if (n < minVocab || w.length < 4 || w.length > 24) continue;
    for (const x of deletes(w, w.length >= 7 ? 2 : 1)) { const l = index.get(x); if (l) l.push(w); else index.set(x, [w]); }
  }
  // Without a dictionary the replies' words are the dictionary, so a real word the replies never used
  // looks misspelled. These cut most of that: a known stem with a suffix is a word ("wasting"), and a
  // typo keeps its first letter and isn't the suggestion with a few letters more or less.
  const SUFFIXES = ['s', 'es', 'ed', 'd', 'ing', 'ly', 'er', 'ers', 'est', 'ness', 'ment', 'ments', 'able', 'ably', 'ion', 'ions', 'al', 'ally', 'ful', 'y'];
  const stemmed = w => SUFFIXES.some(s => w.endsWith(s) && w.length - s.length >= 3 && [w.slice(0, -s.length), `${w.slice(0, -s.length)}e`, w.slice(0, -s.length - 1)].some(x => vocab.has(x)));
  const out = [];
  for (const [w, n] of userCounts) {
    if (w.length < 5 || vocab.has(w) || w.includes("'") || stemmed(w)) continue;
    const max = w.length >= 7 ? 2 : 1;
    let best = null;
    for (const x of deletes(w, max)) for (const c of index.get(x) ?? []) {
      if (c[0] !== w[0] || c.startsWith(w) || w.startsWith(c)) continue;
      const dist = distance(w, c);
      if (dist > max) continue;
      if (!best || dist < best.distance || (dist === best.distance && vocab.get(c) > best.suggest_count)) best = { word: w, count: n, suggest: c, distance: dist, suggest_count: vocab.get(c) };
    }
    if (best) out.push(best);
  }
  return out.sort((a, b) => b.count - a.count || a.word.localeCompare(b.word));
}

export async function importChats(lex, { claude = [], codexDirs = [], claudeExports = [], chatgptExports = [], log = () => {} } = {}) {
  const prompts = [];
  const vocab = new Map();
  const sink = {
    assistant(text) { for (const w of words(text)) vocab.set(w, (vocab.get(w) ?? 0) + 1); },
    user(p) {
      const { text, pasted } = ownWords(p.raw ?? '');
      if (!text || text.startsWith('<')) return;
      prompts.push({ source: p.source, session: String(p.session ?? ''), at: p.at ?? null, cwd: p.cwd ?? null, ...(p.title ? { title: redact(p.title) } : {}), pasted, words: words(text).length, text: redact(text) });
    },
    interrupt(source, session) {
      for (let i = prompts.length - 1; i >= 0; i--) if (prompts[i].source === source && prompts[i].session === session) { prompts[i].interrupted = true; break; }
    },
  };
  for (const d of claude) { log(`Claude Code: ${d}`); await claudeCode(d, sink); }
  for (const d of codexDirs) { log(`Codex: ${d}`); await codex(d, sink); }
  for (const f of claudeExports) { log(`claude.ai export: ${f}`); claudeExport(f, sink); }
  for (const f of chatgptExports) { log(`ChatGPT export: ${f}`); chatgptExport(f, sink); }

  // Earliest first. What the next prompt in the same session says is the only signal of how a prompt went.
  prompts.sort((a, b) => String(a.at).localeCompare(String(b.at)));
  const bySession = new Map();
  for (const p of prompts) { const k = `${p.source}:${p.session}`; (bySession.get(k) ?? bySession.set(k, []).get(k)).push(p); }
  for (const list of bySession.values()) list.forEach((p, i) => {
    const next = list[i + 1];
    p.next = p.interrupted ? 'interrupted' : !next ? 'end' : CORRECTION.test(next.text) ? 'corrected' : 'continued';
    delete p.interrupted;
  });

  const userCounts = new Map();
  const vague = new Map();
  const where = new Map();
  for (const p of prompts) {
    const ws = words(p.text);
    for (const w of ws) { userCounts.set(w, (userCounts.get(w) ?? 0) + 1); if (!where.has(w)) where.set(w, p); }
    const lower = prose(p.text).toLowerCase();
    for (const v of VAGUE) {
      const re = new RegExp(`(?<![a-z])${escape(v)}(?![a-z])`, 'g');
      const n = [...lower.matchAll(re)].length;
      if (!n) continue;
      const s = vague.get(v) ?? { phrase: v, count: 0, prompts: 0, examples: [] };
      s.count += n; s.prompts++;
      if (s.examples.length < 3) { const i = lower.search(re); s.examples.push({ quote: quote(prose(p.text), i, v.length), source: p.source, session: p.session, at: p.at }); }
      vague.set(v, s);
    }
  }
  const spelled = misspellings(userCounts, vocab).slice(0, 300).map(m => {
    const p = where.get(m.word);
    const i = prose(p.text).toLowerCase().indexOf(m.word);
    return { ...m, example: { quote: quote(prose(p.text), Math.max(0, i), m.word.length), source: p.source, session: p.session, at: p.at } };
  });

  // Evidence for existing entries: when each was first and last seen, with the earliest quotes.
  const evidence = {};
  const entries = lex.entries();
  for (const p of prompts) for (const h of matches(prose(p.text), entries)) {
    const e = evidence[h.entry.id] ?? { count: 0, first: p.at, last: p.at, examples: [] };
    e.count++; e.last = p.at;
    if (e.examples.length < 3) e.examples.push({ variant: h.variant.text, quote: h.quote, source: p.source, session: p.session, at: p.at });
    evidence[h.entry.id] = e;
  }

  const clear = prompts.filter(p => p.next === 'continued' && p.words >= 40 && !p.pasted);
  const span = list => list.length ? { prompts: list.length, first: list[0].at, last: list.at(-1).at } : { prompts: 0 };
  const sources = {};
  for (const s of new Set(prompts.map(p => p.source))) sources[s] = span(prompts.filter(p => p.source === s));
  const outcomes = {};
  for (const p of prompts) outcomes[p.next] = (outcomes[p.next] ?? 0) + 1;

  const corpus = path.join(lex.home, 'corpus');
  writeLines(path.join(corpus, 'prompts.jsonl'), prompts);
  writeJSON(path.join(lex.home, 'evidence.json'), evidence);
  const candidates = {
    generated_at: now(),
    note: 'Proposals only. Nothing here is an entry until it is added and confirmed. "clear" means at least 40 words, nothing pasted, and the next prompt in the session did not start with a correction; it is a guess to review, not a judgement.',
    sources, outcomes,
    misspellings: spelled,
    vague: [...vague.values()].sort((a, b) => b.count - a.count),
    clear_prompts: clear.slice(0, 300).map(p => ({ source: p.source, session: p.session, at: p.at, cwd: p.cwd, words: p.words, preview: p.text.slice(0, 280) })),
  };
  writeJSON(path.join(lex.home, 'candidates.json'), candidates);
  return { prompts: prompts.length, vocabulary: vocab.size, sources, outcomes, misspellings: spelled.length, vague: candidates.vague.length, clear: clear.length, evidence: Object.keys(evidence).length };
}

// ---- sync ----

const under = (child, root) => {
  if (!child) return false;
  const a = path.resolve(child).toLowerCase(), b = path.resolve(root).toLowerCase();
  return a === b || a.startsWith(b.endsWith(path.sep) ? b : b + path.sep);
};
export function sync(lex) {
  if (!projectIgnored(lex.root)) throw new Error(`Git does not ignore ${lex.dirs.project}: add /.ai/lexicon/ to .gitignore first`);
  const prompts = readLines(path.join(lex.home, 'corpus', 'prompts.jsonl')).filter(p => under(p.cwd, lex.root));
  const sightings = readLines(path.join(lex.home, 'sightings.jsonl')).filter(s => under(s.project, lex.root));
  writeLines(path.join(lex.dirs.project, 'chats.jsonl'), prompts);
  writeLines(path.join(lex.dirs.project, 'sightings.jsonl'), sightings);
  const project = lex.load('project').entries, global = lex.load('global').entries;
  const conflicts = [];
  for (const p of project) for (const v of p.variants) {
    const g = global.find(e => e.variants.some(x => x.text.toLowerCase() === v.text.toLowerCase()));
    if (g && g.id !== p.id) conflicts.push({ variant: v.text, project: p.id, global: g.id });
  }
  const file = path.join(lex.home, 'projects.json');
  const all = readJSON(file, {});
  all[lex.root] = { name: path.basename(lex.root), synced_at: now(), prompts: prompts.length, sightings: sightings.length, entries: project.length };
  writeJSON(file, all);
  return { root: lex.root, prompts: prompts.length, sightings: sightings.length, project_entries: project.length, conflicts };
}

// ---- command line ----

function parseVariant(s) {
  const parts = s.split(':');
  let ambiguous = false;
  if (parts.at(-1) === 'ambiguous') { ambiguous = true; parts.pop(); }
  const kind = KINDS.includes(parts.at(-1)) ? parts.pop() : 'wrong';
  return { text: parts.join(':').trim(), kind, ...(ambiguous ? { ambiguous } : {}) };
}
const list = v => (v ?? []).flatMap(x => x.split(',')).map(x => x.trim()).filter(Boolean);
const USAGE = `node governance/lexicon.mjs <command>
  where                         the global and project folders
  list [--status S] [--json]    entries, project first
  show ID                       one entry and its evidence
  add --term T --meaning M --variant TEXT[:KIND][:ambiguous]... [--cue W]... [--why W] [--scope global|project] [--id ID]
                                KIND: ${KINDS.join(', ')}; a new entry is proposed
  confirm ID | retire ID | propose ID
  move ID --scope global|project
  check TEXT...                 which entries the text uses (all statuses)
  import [--claude-code DIR]... [--codex DIR]... [--claude-export PATH]... [--chatgpt-export PATH]... [--no-defaults]
                                reads chats into the global corpus; defaults: ~/.claude/projects and ~/.codex
  sync                          copies this project's chats and sightings into its .ai/lexicon
  hook                          Claude Code UserPromptSubmit hook: reads the prompt on stdin, cites confirmed entries`;

export async function main(argv = process.argv.slice(2), env = process.env) {
  const [cmd, ...rest] = argv;
  if (cmd === 'hook') {
    try {
      let input = '';
      for await (const chunk of process.stdin) input += chunk;
      const data = JSON.parse(input || '{}');
      const lex = new Lexicon({ home: globalHome(env), root: path.resolve(env.CLAUDE_PROJECT_DIR ?? data.cwd ?? process.cwd()) });
      const out = hookOutput(lex, data);
      if (out) process.stdout.write(out);
    } catch (error) { process.stderr.write(`lexicon hook: ${error.message}\n`); } // never block a prompt
    return 0;
  }
  const { values: o, positionals: args } = parseArgs({
    args: rest, allowPositionals: true,
    options: {
      term: { type: 'string' }, meaning: { type: 'string' }, variant: { type: 'string', multiple: true }, cue: { type: 'string', multiple: true },
      why: { type: 'string' }, scope: { type: 'string' }, id: { type: 'string' }, status: { type: 'string' }, json: { type: 'boolean' },
      'claude-code': { type: 'string', multiple: true }, codex: { type: 'string', multiple: true }, 'claude-export': { type: 'string', multiple: true },
      'chatgpt-export': { type: 'string', multiple: true }, 'no-defaults': { type: 'boolean' },
    },
  });
  const lex = new Lexicon({ home: globalHome(env), root: projectRoot(env) });
  const print = x => console.log(typeof x === 'string' ? x : JSON.stringify(x, null, 2));
  switch (cmd) {
    case 'where': print({ global: lex.dirs.global, project: lex.dirs.project, project_ignored_by_git: projectIgnored(lex.root) }); return 0;
    case 'list': {
      const es = lex.entries().filter(e => !o.status || e.status === o.status);
      if (o.json) print(es);
      else for (const e of es) print(`${e.status.padEnd(9)} ${e.scope.padEnd(7)} ${e.id}: ${e.variants.map(v => `"${v.text}"`).join(', ')} -> ${e.term}`);
      return 0;
    }
    case 'show': { const e = lex.find(args[0]); print({ ...e, evidence: lex.evidence()[e.id] ?? null }); return 0; }
    case 'add':
      print(lex.add({ id: o.id, term: o.term, meaning: o.meaning, variants: (o.variant ?? []).map(parseVariant), cues: list(o.cue), why: o.why, scope: o.scope ?? 'global' }));
      return 0;
    case 'confirm': print(lex.setStatus(args[0], 'confirmed')); return 0;
    case 'retire': print(lex.setStatus(args[0], 'retired')); return 0;
    case 'propose': print(lex.setStatus(args[0], 'proposed')); return 0;
    case 'move': print(lex.move(args[0], o.scope)); return 0;
    case 'check': {
      const hits = matches(prose(args.join(' ')), lex.entries());
      print(hits.map(h => ({ entry: h.entry.id, status: h.entry.status, variant: h.variant.text, term: h.entry.term })));
      return 0;
    }
    case 'import': {
      const home = os.homedir();
      const defaults = d => (!o['no-defaults'] && fs.existsSync(d) ? [d] : []);
      const result = await importChats(lex, {
        claude: o['claude-code'] ?? defaults(path.join(home, '.claude', 'projects')),
        codexDirs: o.codex ?? defaults(path.join(home, '.codex')),
        claudeExports: o['claude-export'] ?? [], chatgptExports: o['chatgpt-export'] ?? [],
        log: m => process.stderr.write(`reading ${m}\n`),
      });
      print(result);
      return 0;
    }
    case 'sync': print(sync(lex)); return 0;
    default: print(USAGE); return cmd ? 1 : 0;
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().then(code => { process.exitCode = code; }, error => { console.error(error.message); process.exitCode = 1; });
}
