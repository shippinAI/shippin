# Facts

Re-check a fact before repeating it when `verified:` is more than 90 days old.

## The description is the whole trigger

- claim: Agents load only `name` and `description` at startup. The description has to say when the skill applies. The open spec allows 1024 characters. This library caps a description at 300 characters and requires the line to start with "Use when".
- source: https://agentskills.io/specification and https://platform.claude.com/docs/en/agents-and-tools/agent-skills/overview , read 2026-11-19. The local cap is [the library README](../../README.md).
- verified: 2026-11-19

## Workflow skills are a different library

- claim: `.agents/skills/` holds the flat skills agents use while editing this repository. They are not part of the export from `.agents/library/`.
- source: [AGENTS.md](../../../../AGENTS.md) and the `SKILL.md` files under `.agents/skills/`, read 2026-11-19.
- verified: 2026-11-19

## shippin.bot does not load this library yet

- claim: shippin.bot is not started. The roadmap says it is waiting on the repository's name and visibility. Exporting this library writes files on disk and does not sync an assistant.
- source: [docs/roadmap.md](../../../../docs/roadmap.md), the shippin.bot row, read 2026-11-19.
- verified: 2026-11-19
