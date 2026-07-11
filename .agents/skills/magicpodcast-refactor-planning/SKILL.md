---
name: magicpodcast-refactor-planning
description: Prepare a low-risk, reviewable MagicPodcast refactor plan before editing. Use for refactors, cleanup, module boundary changes, dependency restructuring, large component splits, database-related restructuring, or any behavior-preserving change that may affect multiple files or layers.
---

# MagicPodcast Refactor Planning

Plan first and keep the first implementation slice small enough to verify independently.

## Workflow

1. Read `AGENTS.md`, `docs/REFACTORING_ROADMAP.md`, `docs/HUMAN_REVIEW_QUEUE.md`, and `docs/RELEASE_CHECKLIST.md`.
2. Read only the source, tests, and focused documentation related to the proposed refactor.
3. Inspect `git status --short` and preserve pre-existing changes.
4. Describe current behavior and identify the smallest safe seam.
5. Define the files, expected behavior, verification commands, rollback method, and known risks.
6. Stop for explicit approval when the plan changes user-visible behavior, public APIs, database schema or data, search ordering, workflow scheduling, notifications, dependencies, or deployment behavior.
7. Do not edit implementation files during a planning-only request.

## Plan Format

- Goal and current behavior
- First bounded slice
- Impacted files and boundaries
- Behavior change: yes or no
- Verification commands
- Rollback method
- Risks, assumptions, and approval gates

Prefer characterization tests before refactoring behavior that is not already protected.
