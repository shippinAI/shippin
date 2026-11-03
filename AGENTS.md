# Shippin working rules

- Commit directly to `main` and push it; do not create topic branches or pull requests unless asked. Run the relevant checks in CONTRIBUTING.md first, stage explicit paths, and fix a red `main` before adding anything else. Without administrator rights on the repository, open a pull request instead.
- Preserve user work and keep one source owner per concern; see docs/source-of-truth.md. There is one panel, `web/`; do not start another. New variants go through docs/panel-variants.md.
- Do not hardcode installation-specific service addresses, credentials or tenant identifiers. Provide validated configuration and document what is and is not shared.
- Use the project skills under .agents/skills for substantial work. Ground plans in full relevant source files and current evidence.
- Record substantive work, available chat, decisions, checks and failure paths in Git-ignored .ai/data/ via governance/README.md. Do not store secrets there.
- Verify actual behavior and negative paths; report missing services, incomplete checks and environmental limits. Never claim an unimplemented deployment works.
- Never import candidate code without reviewing provenance, license, dependencies, security and tests.
