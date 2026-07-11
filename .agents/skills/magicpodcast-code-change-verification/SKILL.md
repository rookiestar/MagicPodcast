---
name: magicpodcast-code-change-verification
description: Verify MagicPodcast changes before completion or commit by inspecting the diff and running only the checks required for changed backend, frontend, script, documentation, or skill files. Use after modifying code, tests, scripts, configuration, documentation, or project skills in this repository.
---

# MagicPodcast Code Change Verification

Run the repository verification wrapper after edits and before reporting completion.

## Workflow

1. Review `git status --short` and `git diff --stat`.
2. Run:

```bash
.agents/skills/magicpodcast-code-change-verification/scripts/verify.sh
```

3. When a check fails, determine whether it was introduced by the current changes. Do not weaken or skip the check to obtain a passing result.
4. Add narrower checks when the changed behavior is not covered by the wrapper.
5. Run service, browser, database, performance, or production checks only when the task requires them and the user has authorized the operation.

## Report

- List the commands actually run.
- State pass or fail for each relevant check.
- Explain skipped checks and remaining risk.
- State whether the change is safe to commit; do not commit unless explicitly requested.

Never claim an unrun, failed, or partial check passed.
