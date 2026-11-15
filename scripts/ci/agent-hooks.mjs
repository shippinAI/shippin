#!/usr/bin/env node
// Claude Code hooks that keep an AI agent honest about CI. Each reads the hook's JSON on stdin and never
// blocks work it should not: a hook that cannot reach GitHub stays quiet.
//
//   status     SessionStart and UserPromptSubmit: if the latest Checks on main or a queue branch failed,
//              says so first, with the failed jobs, before any other work.
//   watch      PostToolUse on `git push`, run in the background (asyncRewake): follows the Checks run for
//              the pushed commit and, for a queue branch, the Gate that promotes it. On a failure it exits 2
//              with the failed jobs and the end of their logs, which wakes the agent to fix it.
//   test-edit  PreToolUse on file edits: editing an existing test adds the rule about fixing the source.
import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { isTestFile, RULE } from './test-integrity.mjs';

const gh = (args, cwd) => execFileSync('gh', args, { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'], timeout: 30_000 });
const git = (args, cwd) => execFileSync('git', args, { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
const out = (event, context) => process.stdout.write(JSON.stringify({ hookSpecificOutput: { hookEventName: event, additionalContext: context } }));
const sleep = ms => new Promise(r => setTimeout(r, ms));

// The red branches among the latest Checks runs, newest first per branch.
export function redBranches(runs) {
  const latest = new Map();
  for (const r of runs) if (r.status === 'completed' && !latest.has(r.headBranch)) latest.set(r.headBranch, r);
  return [...latest.values()].filter(r => (r.headBranch === 'main' || r.headBranch.startsWith('queue/')) && !['success', 'skipped', 'neutral'].includes(r.conclusion));
}

function status(input) {
  const cwd = input.cwd || process.cwd();
  const cache = path.join(os.tmpdir(), `shippin-ci-status-${Buffer.from(cwd).toString('hex').slice(-32)}.json`);
  let red;
  try {
    const c = JSON.parse(fs.readFileSync(cache, 'utf8'));
    if (Date.now() - c.at < 60_000) red = c.red;
  } catch { /* no cache */ }
  if (!red) {
    const runs = JSON.parse(gh(['run', 'list', '--workflow', 'Checks', '--limit', '30', '--json', 'headBranch,status,conclusion,headSha,url,databaseId'], cwd));
    red = redBranches(runs).map(r => ({ branch: r.headBranch, sha: r.headSha, url: r.url, id: r.databaseId,
      jobs: JSON.parse(gh(['run', 'view', String(r.databaseId), '--json', 'jobs'], cwd)).jobs.filter(j => j.conclusion === 'failure').map(j => j.name) }));
    fs.writeFileSync(cache, JSON.stringify({ at: Date.now(), red }));
  }
  if (!red.length) return;
  const lines = red.map(r => `- ${r.branch} at ${r.sha.slice(0, 12)}: ${r.jobs.join(', ') || 'failed'} (${r.url}; \`gh run view ${r.id} --log-failed\`)`);
  out(input.hook_event_name || 'UserPromptSubmit', `CI IS RED. Fixing it comes before any other work, including this request; tell the person it is red and that you are fixing it first.\n${lines.join('\n')}\n${RULE}`);
}

// The commit a push command sends: the source of "src:dst", else the named branch, else HEAD.
export function pushedRef(command) {
  const words = command.replace(/\s+/g, ' ').trim().split(' ');
  const i = words.indexOf('push');
  const refs = words.slice(i + 1).filter(w => !w.startsWith('-'));
  const spec = refs[1] ?? '';
  if (spec.includes(':')) return { ref: spec.split(':')[0] || 'HEAD', branch: spec.split(':')[1].replace(/^refs\/heads\//, '') };
  return { ref: spec || 'HEAD', branch: spec || null };
}

const PUSH = /\bgit\b[^|;&]*\bpush\b/;

// The folder a push runs in: `git -C dir`, else the last `cd dir` before it, else the session's folder.
// Git Bash paths (/d/work) become Windows paths (D:/work) when Node runs on Windows.
export function pushDir(command, fallback) {
  const before = command.slice(0, command.search(PUSH));
  const dashC = command.match(/\bgit\s+-C\s+("[^"]+"|'[^']+'|\S+)[^|;&]*\bpush\b/);
  const cds = [...before.matchAll(/\bcd\s+("[^"]+"|'[^']+'|[^\s;&|]+)/g)];
  let dir = dashC?.[1] ?? cds.at(-1)?.[1];
  if (!dir) return fallback;
  dir = dir.replace(/^["']|["']$/g, '');
  if (process.platform === 'win32') dir = dir.replace(/^\/([a-zA-Z])\//, (_, d) => `${d.toUpperCase()}:/`);
  return path.isAbsolute(dir) ? dir : path.resolve(fallback, dir);
}

async function waitRun(workflow, sha, cwd, { appear = 180_000, finish = 2_400_000 } = {}) {
  const start = Date.now();
  let run;
  while (Date.now() - start < appear + finish) {
    const runs = JSON.parse(gh(['run', 'list', '--workflow', workflow, '--commit', sha, '--json', 'databaseId,status,conclusion,headBranch,url', '--limit', '5'], cwd));
    run = runs[0];
    if (!run && Date.now() - start > appear) return null;
    if (run?.status === 'completed') return run;
    await sleep(15_000);
  }
  return run ?? null;
}

function failure(kind, run, cwd) {
  let log = '';
  try { log = gh(['run', 'view', String(run.databaseId), '--log-failed'], cwd).split('\n').slice(-60).join('\n'); } catch { /* none */ }
  process.stderr.write(`CI FAILED: ${kind} ended ${run.conclusion} on ${run.headBranch} (${run.url}).\n\nEnd of the failed logs:\n${log}\n\n${RULE}\nFix it now, before any other work: read the failure, find the cause in the source, fix it, run the check locally, and push again.\n`);
  process.exit(2);
}

async function watch(input) {
  const command = input.tool_input?.command ?? '';
  if (!PUSH.test(command)) return;
  const cwd = pushDir(command, input.cwd || process.cwd());
  const { ref, branch } = pushedRef(command.slice(command.search(PUSH)));
  const sha = git(['rev-parse', `${ref}^{commit}`], cwd);
  const checks = await waitRun('Checks', sha, cwd);
  if (!checks) return;
  if (checks.conclusion !== 'success') failure('Checks', checks, cwd);
  if ((branch ?? checks.headBranch).startsWith('queue/')) {
    const gate = await waitRun('Gate', sha, cwd, { appear: 120_000, finish: 600_000 });
    if (gate && gate.conclusion !== 'success') failure('Gate (promotion to main)', gate, cwd);
  }
}

function testEdit(input) {
  const file = input.tool_input?.file_path ?? input.tool_input?.notebook_path ?? '';
  if (!file || !isTestFile(file) || !fs.existsSync(file)) return;
  out('PreToolUse', `You are changing an existing test (${path.basename(file)}). Adding tests and checks is welcome. If this edit removes, loosens or skips a check so that something passes, stop: ${RULE}`);
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  let raw = '';
  for await (const chunk of process.stdin) raw += chunk;
  let input = {};
  try { input = JSON.parse(raw || '{}'); } catch { /* not JSON */ }
  const mode = process.argv[2];
  try {
    if (mode === 'status') status(input);
    else if (mode === 'watch') await watch(input);
    else if (mode === 'test-edit') testEdit(input);
  } catch (error) {
    // GitHub unreachable or gh signed out: say nothing rather than guess.
    if (process.env.SHIPPIN_HOOK_DEBUG) process.stderr.write(`ci hook ${mode}: ${error.message}\n`);
  }
}
