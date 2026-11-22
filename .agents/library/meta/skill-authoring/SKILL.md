---
name: shippin-meta-skill-authoring
description: Use when drafting, splitting, reviewing, renaming, or refreshing a skill in the canonical assistant library. Covers the trigger line, sibling facts and trigger tests, pull-request review, and re-checking dated facts. Not for ordinary product code.
---

# Skill authoring

Draft a skill from a repeated instruction or a correction. One skill does one job. If the note covers two jobs, split it.

1. Copy [the template](https://github.com/shippinAI/shippin/tree/main/.agents/library/_template) to `.agents/library/<area>/<skill>/`. Both names are lowercase words joined by single hyphens.
2. Set `name` to `shippin-<area>-<skill>`. That string is the folder name after export, and it has to match the file. The rules are in [the library README](https://github.com/shippinAI/shippin/blob/main/.agents/library/README.md).
3. Write `description` as one line that starts with `Use when`. Name the intent that should load the skill, and the nearby intent that should not. The description is the only trigger. This body is not visible until it matches.
4. Put the procedure here. It has to stand alone. Link live docs and [facts.md](https://github.com/shippinAI/shippin/blob/main/.agents/library/meta/skill-authoring/facts.md) by HTTPS URL so the links survive export. Do not paste hostnames, credentials, tenant ids, or a second copy of a page that will change.
5. Put each dated claim in `facts.md` with a source and one `verified:` date. Put prompts that should and should not trigger the skill in `tests.md`.
6. Use the repository's current review process and the owner's task authorization. Review the trigger, should and should-not prompts, and each new fact before merge. Do not install the export into an assistant without authorization for that installation.
7. When you next touch the skill, re-check facts older than 90 days. Drop anything the source no longer says.

Run `node scripts/skills/lint.mjs` from the repository root before the review.
