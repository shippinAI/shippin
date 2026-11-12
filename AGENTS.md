# Shippin working rules

- Commit directly to `main` and push it; do not create topic branches or pull requests unless asked. Run the relevant checks in CONTRIBUTING.md first, stage explicit paths, and fix a red `main` before adding anything else. Without administrator rights on the repository, open a pull request instead.
- Preserve user work and keep one source owner per concern; see docs/source-of-truth.md. There is one panel, `web/`; do not start another. New variants go through docs/panel-variants.md.
- Before substantial implementation, map the packet to requirement IDs in docs/objective.md and the active stage in docs/roadmap.md. Reconcile older plans with those contracts. Internal operator ownership/staff grants and tenant workspace/project access must remain distinct; a dependency container or form alone does not complete a managed-service capability.
- Do not hardcode installation-specific service addresses, credentials or tenant identifiers. Provide validated configuration and document what is and is not shared.
- Use the project skills under .agents/skills for substantial work. Ground plans in full relevant source files and current evidence.
- Record substantive work, available chat, decisions, checks and failure paths in Git-ignored .ai/data/ via governance/README.md. Do not store secrets there.
- Verify actual behavior and negative paths; report missing services, incomplete checks and environmental limits. Never claim an unimplemented deployment works.
- Never import candidate code without reviewing provenance, dependencies, security and tests.
- During an authorized initial installation, paste the current one-time setup code printed by the running Shippin panel directly into the active user chat so the owner can complete setup. Never record the code in `.ai/data`, source files, or commits.
