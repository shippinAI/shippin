# Agent work records

The local CLI records plans, journal entries and user/assistant text explicitly under Git-ignored `.ai/data/sessions/YYYY/MM/<uuid>/`. It does not automatically capture chats, encrypt records, prove truthfulness or replace independent behavioral testing. Keep credentials out.

Run `npm ci && npm test` in this directory. From the repository root, use `node governance/session.mjs create OBJECTIVE CAPTURE_SCOPE`, then `chat`, `plan`, `journal`, `resume` or `list`; invoke without arguments for usage. Plans use the versioned schema in `schemas/`; validate a plan and initial source hashes with `node governance/validate.mjs plan PATH` and `node governance/validate.mjs baseline PATH`. Review evidence, failure paths and handoff state. See `.agents/skills/` for project-specific workflows.

## Lexicon

`node governance/lexicon.mjs` keeps one developer's own terms: wrong, misspelled, vague or overloaded words, each bridged once to a preferred term and cited after that. It is personal and never in a repository:

- **Global**, `$AI_LEXICON_HOME` or `~/.ai/lexicon/`: the glossary for every project, the chat corpus (`corpus/prompts.jsonl`, earliest first, secrets redacted), the evidence for each entry, candidates to review and the sightings the hook records.
- **Project**, `.ai/lexicon/` (Git-ignored; the tool refuses to write there otherwise): terms only this project uses, which win over global ones here, and `sync`'s copy of this project's chats and sightings.

An entry is `proposed` until the person confirms it (`confirm ID`). `import` reads Claude Code (`~/.claude/projects`, without sub-agents or SDK sessions), Codex (`~/.codex`), and a claude.ai or ChatGPT export (`--claude-export`, `--chatgpt-export`, the unzipped folder). It writes candidates (misspellings against the words the replies use, vague words, prompts that were not followed by a correction) and never adds or confirms an entry. `hook` is a Claude Code `UserPromptSubmit` hook. It cites confirmed entries the prompt uses, ignores pasted text and code, and never blocks a prompt:

```json
{ "hooks": { "UserPromptSubmit": [ { "hooks": [ { "type": "command", "command": "node \"$CLAUDE_PROJECT_DIR/governance/lexicon.mjs\" hook" } ] } ] } }
```

Put that in `.claude/settings.local.json` for one project, or in `~/.claude/settings.json` with the script's absolute path for every project. The format can later become COACH-01's notes on a person, which belong to that person.
