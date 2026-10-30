# Agent work records

The local CLI records plans, journal entries and user/assistant text explicitly under Git-ignored `.ai/data/sessions/YYYY/MM/<uuid>/`. It does not automatically capture chats, encrypt records, prove truthfulness or replace independent behavioral testing. Keep credentials out.

Run `npm ci && npm test` in this directory. From the repository root, use `node governance/session.mjs create OBJECTIVE CAPTURE_SCOPE`, then `chat`, `plan`, `journal`, `resume` or `list`; invoke without arguments for usage. Plans use the versioned schema in `schemas/`; validate a plan and initial source hashes with `node governance/validate.mjs plan PATH` and `node governance/validate.mjs baseline PATH`. Review evidence, failure paths and handoff state. See `.agents/skills/` for project-specific workflows.
