---
name: evidence-execution
description: Execute and verify a grounded Shippin.Cloud handoff, recording actual results and carrying unfinished work forward.
---

# Evidence execution

Read the accepted handoff and relevant repository instructions. Validate the plan
and verify baseline source hashes using `governance/README.md` before acting. Read
the complete source files needed for the current step. If evidence is stale or a
step conflicts with reality or current user instructions, stop the dependent step,
record the discrepancy and update the plan; do not silently improvise.

Work in the stated order and scope, preserve existing changes, and journal progress,
commands actually run, exit status, artifacts, failed or skipped checks and next
actions in the `.ai/data/` session. Never mark a task done from an agent's assertion. Run independent checks
appropriate to the change; use isolated test resources when tests mutate state. A
validator proves structure and hashes only, not correctness or truthful execution.
Do not alter tests or acceptance criteria just to turn a failure green. Record any
legitimate check change and why. Report incomplete work and environmental limits.

For each plan step record evidence of the actual result, including incomplete or
blocked steps. Verify meaningful behavior across the full intended workflow and
negative paths where feasible. Inspect changed tests and acceptance checks, compare
the final diff and resulting state to the plan, and use independent CI or another
reviewer where available. A small green unit test alone does not prove the objective.

When AgentHub is available, use its existing task, fence and journal protocol rather
than inventing a second live task store. Until then, keep the local handoff and journal
with the work; neither a file nor an instruction forces every external agent to comply.
