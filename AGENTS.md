# Shippin working rules

- Preserve user work and keep one source owner per concern; see docs/source-of-truth.md.
- Do not hardcode installation-specific service addresses, credentials or tenant identifiers. Provide validated configuration and document what is and is not shared.
- Use the project skills under .agents/skills for substantial work. Ground plans in full relevant source files and current evidence.
- Record substantive work, available chat, decisions, checks and failure paths in Git-ignored .ai/data/ via governance/README.md. Do not store secrets there.
- Verify actual behavior and negative paths; report missing services, incomplete checks and environmental limits. Never claim an unimplemented deployment works.
- Never import candidate code without reviewing provenance, license, dependencies, security and tests.
