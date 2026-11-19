# Assistant skill library

This is the canonical, reviewed library of intent-triggered checklists for Jesse's assistants (Grok Bot / Clyffy, and later shippin.bot). It is the source those assistants should load. Nothing in this directory is synced anywhere by itself.

The flat workflow skills that agents use while editing this repository stay in [`.agents/skills/`](../skills/). The host diagnostic stays in [`agent/`](../../agent/README.md). This library is a third thing: nested in git, exported flat for an assistant.

## Why it is not `agent/skills/`

[`agent/`](../../agent/README.md) owns the read-only Incus and Proxmox diagnostic, and the `agent` CI job tests that Go module. [Source ownership](../../docs/source-of-truth.md) gives that directory one concern. Putting assistant prompts there would give it a second owner.

`.agents/skills/` is already a flat library Cursor loads for work in this repo (`costar-alignment`, `evidence-execution`, `grounded-handoff`). Those skills have no `facts.md` or `tests.md`, and they are not exported. Nesting this catalog inside that folder would mix two shapes and two consumers.

## How an assistant loads a skill

The consuming library is flat. One folder per slug holds a `SKILL.md`. At startup the assistant sees only the `name` and the one-line `description`. It loads the body when that description matches the conversation. It does not load sibling files unless the body links one and the assistant opens it. `SKILL.md` has to stand alone, and it has to stay short.

That is the same progressive disclosure as the Agent Skills format: metadata first, instructions when triggered, further files only if linked. See [the spec](https://agentskills.io/specification), [Anthropic's overview](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/overview), and [the research note](https://www.anthropic.com/research/equipping-agents-for-the-real-world-with-agent-skills) (read 2026-11-19).

## Layout

```text
.agents/library/
  README.md
  _template/
  <area>/<skill>/
    SKILL.md
    facts.md
    tests.md
```

`<area>` and `<skill>` are lowercase words separated by single hyphens (`infra`, `change-control`). `_template/` is the copy source. It is linted and it is not exported.

| File | Who it is for | What it holds |
| --- | --- | --- |
| `SKILL.md` | The assistant, after the description matches | YAML frontmatter (`name`, `description`) and a short checklist. Link live sources. |
| `facts.md` | Authors and reviewers | Dated claims. Each `##` fact has a source and one `verified: YYYY-MM-DD`. |
| `tests.md` | Authors and reviewers | Prompts that should and should not trigger the skill. |

## Names

The flat export name is `shippin-<area>-<skill>`. The frontmatter `name` is that string, not the short directory name.

The Agent Skills spec says `name` must match the directory the assistant loads. Export writes `.agents/library/infra/change-control/SKILL.md` to `shippin-infra-change-control/SKILL.md` without editing it, so the loaded folder and `name` match. The spec allows 64 characters. `shippin-` plus the area, a hyphen, and the skill must fit.

`description` is the trigger. It is one line, it starts with `Use when`, and it says when the skill applies, including the words a person would actually use. Write it in a form that can sit next to every other description in the system prompt ([authoring guidance](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices), [trigger testing](https://github.com/agentskills/agentskills/blob/main/docs/skill-creation/optimizing-descriptions.mdx)). The spec allows 1024 characters. This library allows 300, because every description is loaded for every conversation.

Frontmatter is only `name` and `description`, one line each. A surrounding pair of double quotes is optional. Other spec fields (`license`, `compatibility`, `metadata`) are rejected here so the file stays what this consumer reads.

## Size

`scripts/skills/lint.mjs` is the contract:

- `description`: 1–300 characters, one line, starts with `Use when `.
- Body: at most 120 lines and 6000 bytes. The spec's comfort zone is 500 lines; these checklists are shorter because the consumer does not load the siblings automatically.
- `name`: the flat export name, at most 64 characters, lowercase letters, digits, and single hyphens.
- `facts.md` and `tests.md` sit beside `SKILL.md`. `tests.md` has `## Should trigger` and `## Should not trigger`, each with at least one prompt.
- A `verified:` date must be a real calendar day. A date more than 90 days old prints a warning and does not fail the check. A future or unparseable date fails. Re-check a warned fact before relying on it.
- Secret-shaped text fails the check: AWS access key ids, private-key blocks, GitHub tokens (`ghp_`, `github_pat_`), Slack tokens, `sk-` style keys, and a quoted value assigned to an api key, secret, token, or password.

## Anti-context-rot

- **One job.** If a skill covers two decisions, split it.
- **Link the live source.** Point at the repo path or URL that is true now. Do not paste a second copy of a page, a host list, or a status table into `SKILL.md`.
- **Date the claims that rot.** Versions, "how main moves", and "what is not built yet" go in `facts.md` with `verified:` and a source. The checklist in `SKILL.md` is the standing procedure.
- **No secrets, ever.** No tokens, keys, connection strings, or one-time setup codes. Give a file path or a setting name. This includes `_template/` and `tests.md`.
- **No installation identities.** Addresses, domains, and tenant ids of one installation stay out, same as the rest of this repository.
- **Re-research on a schedule.** Before using a fact older than 90 days, open the source again and move `verified:` or delete the fact. When a skill is wrong in conversation, fix the skill in a pull request rather than patching around it in chat.
- **Review the trigger.** A description change changes when the assistant loads the skill. CI checks shape, not judgment. The should and should-not prompts are how a reviewer sees the intended boundary ([trigger guidance](https://github.com/agentskills/agentskills/blob/main/docs/skill-creation/optimizing-descriptions.mdx)).

## Change control for this library

Skills in this tree change through a pull request. The rest of the repository usually lands on `main` through `queue/main` ([CONTRIBUTING.md](../../CONTRIBUTING.md)). This library is stricter: the text is the behavior, and CI does not run an assistant. A person reads the description, the trigger prompts, and any new fact before merge.

Do not export into an assistant until that review has landed. The export script only writes files on disk.

## Commands

From the repository root:

```sh
node scripts/skills/lint.mjs
node --test scripts/skills/skills.test.mjs
node scripts/skills/export.mjs /path/to/empty-output-dir
```

`scripts/check.sh` and the `skills` job in [Checks](../../.github/workflows/checks.yml) run the linter and the tests. The export writes one directory per flat name and copies `SKILL.md` unchanged. It skips `_template/`. It does not delete extra folders in the output, it does not copy `facts.md` or `tests.md`, and it does not talk to an assistant. It refuses to write inside this library, and it refuses to export a library the linter rejects.

## What is here

- [`_template/`](_template/SKILL.md) — copy this when drafting a skill.
- [`meta/skill-authoring/`](meta/skill-authoring/SKILL.md) — how a skill is drafted, reviewed, and kept current.
- [`infra/change-control/`](infra/change-control/SKILL.md) — standing rules for infrastructure, deploy, and git-history changes.
