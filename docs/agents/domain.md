# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Read when domain semantics matter

- Read the relevant parts of [CONTEXT.md](../../CONTEXT.md) for product terminology, domain behavior, or changes across module boundaries.
- Read decisions under [docs/adr/](../adr/) when they govern the area being changed. Routine wording, formatting, or unrelated local fixes do not require a domain-document tour.

Follow the task-based reading rules in [AGENTS.md](../../AGENTS.md). Missing optional documentation is not a setup failure; create or change domain documentation only when resolving a relevant term or decision is within the task.

## Use the glossary's vocabulary

When your output names a domain concept (in an issue title, a refactor proposal, a hypothesis, a test name), use the term as defined in `CONTEXT.md`. Don't drift to synonyms the glossary explicitly avoids.

If the concept you need isn't in the glossary yet, that's a signal: either you're inventing language the project doesn't use (reconsider) or there's a real gap (note it for `/domain-modeling`).

## Flag ADR conflicts

If a proposed change contradicts an existing ADR, identify the actual ADR and the conflicting behavior. Resolve any required decision before changing that behavior; do not treat example ADR numbers or old proposals as current policy.
