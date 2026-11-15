import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync, spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { Lexicon, matches, hookOutput, importChats, sync, redact, ownWords, distance, misspellings, words } from '../lexicon.mjs';

const cli = path.join(path.dirname(fileURLToPath(import.meta.url)), '..', 'lexicon.mjs');
const tmp = () => fs.mkdtempSync(path.join(os.tmpdir(), 'lexicon-'));
function project({ ignore = true } = {}) {
  const root = tmp();
  execFileSync('git', ['init', '-q'], { cwd: root });
  if (ignore) fs.writeFileSync(path.join(root, '.gitignore'), '/.ai/lexicon/\n');
  return root;
}
const tenant = { term: 'multi-tenant', meaning: 'Many customers share one installation, each kept apart.', variants: [{ text: 'multi-tenet', kind: 'misspelling' }, { text: 'tenet', kind: 'wrong', ambiguous: true }], cues: ['customer', 'cloud', 'panel'] };

test('an ambiguous variant counts only near one of its cues', () => {
  const lex = new Lexicon({ home: tmp(), root: project() });
  lex.add(tenant);
  const es = lex.entries();
  assert.equal(matches('the multi-tenet panel', es).length, 1);
  assert.equal(matches('a core tenet of design', es).length, 0);
  assert.equal(matches('each tenet gets a customer database', es).length, 1);
  assert.equal(matches('multi tenet cloud', es).length, 1, 'a space or a dash both match, and "tenet" inside it is the same word');
  assert.equal(matches('multi-tenant is right', es).length, 0);
  assert.equal(matches('the multi tenet cloud', [{ ...es[0], variants: [...es[0].variants, { text: 'multi tenet', kind: 'misspelling' }] }]).length, 1, 'one span, two variants');
  assert.equal(matches('a core tenet of how we write every one of these long sentences, about the cloud', es).length, 0, 'the cue is too far away');
});

test('entries are proposed, confirmed, moved, and project ones win', () => {
  const root = project();
  const lex = new Lexicon({ home: tmp(), root });
  const e = lex.add(tenant);
  assert.equal(e.status, 'proposed');
  assert.throws(() => lex.add(tenant), /exists/);
  assert.throws(() => lex.add({ term: 'x', meaning: 'y', variants: [{ text: 'tenet', kind: 'wrong' }] }), /variant of/);
  assert.throws(() => lex.add({ term: 'z', meaning: 'y', variants: [{ text: 'zz', kind: 'nope' }] }), /kind/);
  assert.throws(() => lex.add({ term: 'q', meaning: 'y', variants: [{ text: 'qq', kind: 'wrong', ambiguous: true }] }), /needs cues/);
  assert.equal(lex.setStatus('multi-tenant', 'confirmed').status, 'confirmed');
  lex.move('multi-tenant', 'project');
  assert.equal(lex.find('multi-tenant').scope, 'project');
  assert.equal(lex.load('global').entries.length, 0);
  assert.ok(fs.existsSync(path.join(root, '.ai', 'lexicon', 'lexicon.json')));
});

test('nothing is written into a project that Git does not ignore', () => {
  const lex = new Lexicon({ home: tmp(), root: project({ ignore: false }) });
  assert.throws(() => lex.add({ ...tenant, scope: 'project' }), /add \/\.ai\/lexicon\/ to \.gitignore/);
  assert.throws(() => sync(lex), /gitignore/);
});

test('the hook cites confirmed entries only, records the sighting, and ignores pasted text', () => {
  const home = tmp();
  const lex = new Lexicon({ home, root: project() });
  lex.add(tenant);
  assert.equal(hookOutput(lex, { prompt: 'the multi-tenet cloud' }), '', 'proposed entries are not cited');
  lex.setStatus('multi-tenant', 'confirmed');
  const out = JSON.parse(hookOutput(lex, { prompt: 'the multi-tenet cloud and the multi-tenet panel', session_id: 's1' }));
  assert.equal(out.hookSpecificOutput.hookEventName, 'UserPromptSubmit');
  assert.match(out.hookSpecificOutput.additionalContext, /"multi-tenet" means multi-tenant \(misspelling\)/);
  assert.equal(out.hookSpecificOutput.additionalContext.split('multi-tenet"').length - 1, 1, 'cited once');
  assert.equal(fs.readFileSync(path.join(home, 'sightings.jsonl'), 'utf8').trim().split('\n').length, 2);
  assert.equal(hookOutput(lex, { prompt: 'ok <pasted_content id="1">the multi-tenet cloud</pasted_content id="1">' }), '');
  assert.equal(hookOutput(lex, { prompt: 'see `multi-tenet` in code' }), '', 'code is not prose');
});

test('the hook never blocks a prompt', () => {
  const home = tmp();
  fs.writeFileSync(path.join(home, 'lexicon.json'), 'not json');
  const r = spawnSync(process.execPath, [cli, 'hook'], { input: '{"prompt":"hello"}', env: { ...process.env, AI_LEXICON_HOME: home, CLAUDE_PROJECT_DIR: tmp() }, encoding: 'utf8' });
  assert.equal(r.status, 0);
  assert.equal(r.stdout, '');
  const bad = spawnSync(process.execPath, [cli, 'hook'], { input: 'garbage', env: { ...process.env, AI_LEXICON_HOME: tmp() }, encoding: 'utf8' });
  assert.equal(bad.status, 0);
});

test('secrets are redacted and tool wrappers are not the person\'s words', () => {
  const s = redact('use Bearer abcdefghijklmnopqrstuv and password=hunter22 and ghp_abcdefghijklmnopqrstu and sk-ant-0123456789abcdefghij');
  assert.doesNotMatch(s, /abcdefghijklmnop|hunter22|0123456789abcdef/);
  assert.match(s, /password=\[REDACTED\]/);
  const w = ownWords('<system-reminder>ignore me</system-reminder>real words <pasted_content id="9">theirs</pasted_content id="9">');
  assert.equal(w.text, 'real words');
  assert.equal(w.pasted, true);
  assert.deepEqual(words('see https://x.example/a and D:\\dir\\file and `code_here` plus words'), ['see', 'and', 'and', 'plus', 'words']);
});

test('misspellings are found against the words the replies use', () => {
  assert.equal(distance('habbit', 'habit'), 1);
  assert.equal(distance('teh', 'the'), 1);
  const vocab = new Map([['habit', 40], ['substitute', 30], ['tenant', 50], ['other', 90], ['waiting', 40], ['waste', 9], ['constant', 20]]);
  const found = misspellings(new Map([['habbit', 2], ['substite', 1], ['tenant', 3], ['zzzzzz', 1], ['mother', 4], ['wasting', 3], ['constantly', 2]]), vocab);
  assert.deepEqual(found.map(f => [f.word, f.suggest]), [['habbit', 'habit'], ['substite', 'substitute']]);
});

function fixtures(dir) {
  const cc = path.join(dir, 'claude', 'd--proj');
  fs.mkdirSync(path.join(cc, 'sess-1', 'subagents'), { recursive: true });
  const line = r => JSON.stringify(r);
  fs.writeFileSync(path.join(cc, 'sess-1.jsonl'), [
    line({ type: 'user', timestamp: '2026-09-23T10:00:00Z', cwd: 'D:\\work\\proj', entrypoint: 'claude-vscode', message: { content: 'build the multi-tenet panel so each customer gets a database, with a migration, a test of the negative path, and a short note in the docs saying what is shared and what is not between customers in one installation, then check it on the review deploy' } }),
    line({ type: 'assistant', message: { content: [{ type: 'text', text: 'Done. Each tenant gets a database and the habit of checking holds.' }] } }),
    line({ type: 'user', timestamp: '2026-09-23T10:05:00Z', message: { content: [{ type: 'tool_result', content: 'ignored' }] } }),
    line({ type: 'user', timestamp: '2026-09-23T10:06:00Z', cwd: 'D:\\work\\proj', message: { content: 'no, that is wrong' } }),
    line({ type: 'user', timestamp: '2026-09-23T10:07:00Z', cwd: 'D:\\work\\proj', isMeta: true, message: { content: 'meta' } }),
    line({ type: 'user', timestamp: '2026-09-23T10:08:00Z', cwd: 'D:\\work\\proj', isSidechain: true, message: { content: 'sidechain' } }),
    line({ type: 'user', timestamp: '2026-09-23T10:09:00Z', cwd: 'D:\\work\\proj', entrypoint: 'sdk-cli', message: { content: 'automated' } }),
    'not json',
  ].join('\n'));
  fs.writeFileSync(path.join(cc, 'sess-1', 'subagents', 'agent-a.jsonl'), line({ type: 'user', message: { content: 'sub-agent prompt' } }));
  const cx = path.join(dir, 'codex');
  fs.mkdirSync(path.join(cx, 'sessions', '2026', '09', '25'), { recursive: true });
  fs.writeFileSync(path.join(cx, 'sessions', '2026', '09', '25', 'rollout-x.jsonl'), [
    line({ type: 'session_meta', payload: { id: 'cx-1', cwd: 'D:\\work\\other' } }),
    line({ type: 'response_item', payload: { type: 'message', role: 'assistant', content: [{ type: 'output_text', text: 'habit habit habit habit habit substitute substitute substitute substitute substitute' }] } }),
  ].join('\n'));
  fs.writeFileSync(path.join(cx, 'history.jsonl'), [
    line({ session_id: 'cx-1', ts: 1790380538, text: 'fix my bad habbit with token=abc123secretvalue' }),
  ].join('\n'));
  const ca = path.join(dir, 'claude-ai');
  fs.mkdirSync(ca);
  fs.writeFileSync(path.join(ca, 'conversations.json'), JSON.stringify([{ uuid: 'c1', name: 'Early', chat_messages: [
    { sender: 'human', created_at: '2025-01-02T00:00:00Z', text: 'first early prompt' },
    { sender: 'assistant', created_at: '2025-01-02T00:00:05Z', text: 'reply words' },
  ] }]));
  const gpt = path.join(dir, 'gpt.json');
  fs.writeFileSync(gpt, JSON.stringify([{ id: 'g1', title: 'GPT', mapping: {
    a: { message: { author: { role: 'user' }, create_time: 1700000000, content: { parts: ['the earliest of all'] } } },
    b: { message: { author: { role: 'assistant' }, create_time: 1700000005, content: { parts: ['answer'] } } },
    c: { message: null },
  } }]));
  return { claude: [path.join(dir, 'claude')], codexDirs: [cx], claudeExports: [ca], chatgptExports: [gpt] };
}

test('import reads every source, earliest first, and only proposes', async () => {
  const home = tmp(), root = project();
  const lex = new Lexicon({ home, root });
  lex.add(tenant);
  const r = await importChats(lex, fixtures(tmp()));
  assert.deepEqual(Object.keys(r.sources).sort(), ['chatgpt', 'claude-ai', 'claude-code', 'codex']);
  const prompts = fs.readFileSync(path.join(home, 'corpus', 'prompts.jsonl'), 'utf8').trim().split('\n').map(JSON.parse);
  assert.deepEqual(prompts.map(p => p.text.slice(0, 12)), ['the earliest', 'first early ', 'build the mu', 'no, that is ', 'fix my bad h']);
  assert.equal(prompts.find(p => p.source === 'claude-code').next, 'corrected', 'followed by a correction');
  assert.ok(!prompts.some(p => /sidechain|automated|meta|sub-agent|ignored/.test(p.text)));
  assert.doesNotMatch(JSON.stringify(prompts), /abc123secretvalue/);
  assert.equal(prompts.find(p => p.source === 'codex').cwd, 'D:\\work\\other');
  const c = JSON.parse(fs.readFileSync(path.join(home, 'candidates.json'), 'utf8'));
  assert.ok(c.misspellings.some(m => m.word === 'habbit' && m.suggest === 'habit'));
  const ev = JSON.parse(fs.readFileSync(path.join(home, 'evidence.json'), 'utf8'));
  assert.equal(ev['multi-tenant'].first, '2026-09-23T10:00:00Z');
  assert.equal(lex.find('multi-tenant').status, 'proposed', 'import never confirms');
});

test('sync copies only this project\'s chats into its ignored folder', async () => {
  const home = tmp(), root = project();
  const lex = new Lexicon({ home, root });
  const f = fixtures(tmp());
  const prompts = [{ source: 'claude-code', session: 's', at: '2026-09-23T00:00:00Z', cwd: path.join(root, 'web'), text: 'mine' }, { source: 'codex', session: 't', at: '2026-09-24T00:00:00Z', cwd: path.join(root + '-other'), text: 'not mine' }];
  fs.mkdirSync(path.join(home, 'corpus'), { recursive: true });
  fs.writeFileSync(path.join(home, 'corpus', 'prompts.jsonl'), prompts.map(p => JSON.stringify(p)).join('\n') + '\n');
  assert.ok(f);
  const r = sync(lex);
  assert.equal(r.prompts, 1);
  assert.equal(fs.readFileSync(path.join(root, '.ai', 'lexicon', 'chats.jsonl'), 'utf8').includes('not mine'), false);
  assert.ok(JSON.parse(fs.readFileSync(path.join(home, 'projects.json'), 'utf8'))[root]);
});
