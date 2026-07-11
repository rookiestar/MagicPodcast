---
name: magicpodcast-database-change-guard
description: Gate and verify MagicPodcast operations that may write, replace, migrate, import, synchronize, repair, or delete real SQLite data. Use before running database migrations, restore scripts, manual SQL, backend/cmd/maint commands, import or sync jobs, or any command whose target database is not an isolated test database.
---

# MagicPodcast Database Change Guard

Treat the target database and data-changing command as untrusted until their scope and recovery path are proven.

## Workflow

1. Read `AGENTS.md`, `docs/BACKUP_RECOVERY.md`, `docs/migration/MIGRATION_GUIDE.md`, `docs/HUMAN_REVIEW_QUEUE.md`, and `backend/cmd/README.md`.
2. Inspect the exact command, SQL, script, target database path, environment variables, and service state.
3. Classify the operation as read-only, isolated-test write, or real-data write.
4. For a real-data write, stop unless the user explicitly authorized that exact operation and target.
5. Before an authorized write:
   - verify the target database path;
   - create and verify a current backup;
   - stop services when required by the documented procedure;
   - state the rollback method;
   - use a dry run or temporary copy when the command supports it.
6. Execute only the approved command. Do not broaden into adjacent cleanup or repair.
7. Verify database integrity, expected row-level effects, service health when relevant, and backup availability.

## Hard Stops

- Never infer authorization for real-data writes from a request to inspect or diagnose.
- Never use `--force`, `--no-safety-backup`, destructive SQL, or direct file replacement to bypass a failed guard.
- Never run `backend/cmd/migrate`, `backend/cmd/maint/`, restore, import, sync, or repair commands against a real database without explicit approval.
- Never delete databases, backups, WAL/SHM files, local configuration, or logs unless explicitly requested.
- Never report success from command exit status alone; verify the resulting data and integrity.

## Report

- Approved command and target
- Backup and rollback evidence
- Verification performed and result
- Exact data impact
- Remaining risk or follow-up
