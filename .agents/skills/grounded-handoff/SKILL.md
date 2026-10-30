---
name: grounded-handoff
description: Ground and write a verifiable implementation handoff for substantial Shippin.Cloud work, especially when another agent will execute it.
---

# Grounded handoff

Read the user's objective, current repository instructions and relevant whole source
files, including call sites, tests and configuration. Identify dirty work and ownership;
never import or replace code merely because a similarly named file exists. Record
what was inspected, its digest, observed behavior, and uncertainty. Research current
primary sources when facts are unstable, niche or unknown; cite links and dates.

Use CO-STAR as a lightweight completeness check: context, objective, style, tone,
audience and response format. It is not proof that an answer is correct. First infer
what the evidence supports; then ask only decision-changing questions, in plain
language, with a short reason and sensible options. Continue independent work.

For substantial handoffs use `governance/README.md` and its plan schema, saving
the plan in the relevant `.ai/data/` session. Specify
the objective, boundaries, current state and source hashes, ordered concrete steps,
expected results, checks, failure paths and the next action. Checks must establish
the desired behavior, not just the presence of words or files. Separate known facts,
assumptions and unresolved decisions. Do not invent command results, claim unread
files were reviewed, or declare missing infrastructure deployed. Give a successor
enough context to act without guessing, while requiring it to stop on stale evidence
or a contradiction rather than obeying a harmful or impossible instruction.
Review existing boundary ownership, public contracts, callers and tests before
creating a new folder or schema. Prefer one owner per concern; describe migration
and compatibility rather than silently introducing a duplicate authority. Tie
every objective and behavior to an acceptance check that observes end-to-end
outcomes and relevant failure or recovery paths, in addition to focused tests.
