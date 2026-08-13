---
name: magicpodcast-data-profile
description: Operate MagicPodcast local data profiles through the repository command. Use for checking the current Fixture or Snapshot, switching to an explicitly requested profile or Fixture scenario, diagnosing profile readiness, or carrying out a separately authorized Snapshot refresh.
---

# MagicPodcast data profile

Use `./scripts/data-profile.sh` as the only operation entrypoint. Read
`docs/DATA_PROFILES.md` when command details or failure recovery are needed.

## Workflow

1. Run `./scripts/data-profile.sh --json status`.
2. Report the current Profile and non-sensitive metadata when the user only
   asks for status, advice, or network-change behavior.
3. For an explicit local switch, confirm the requested target from the status
   result, then run exactly one:
   - `./scripts/data-profile.sh use fixture [scenario]`
   - `./scripts/data-profile.sh use snapshot [latest|ID]`
4. Run `./scripts/data-profile.sh --json status`, query `/ready` on the
   configured Profile port, and query one representative `/api/v1` endpoint.
5. Report Profile, schema, Fixture version/scenario/anchor or Snapshot
   ID/capture time, readiness, and whether the API data matches the target.

If the request does not explicitly select a target, stop after status and
advice. Network changes, API failures, startup failures, and Snapshot age never
select or refresh a Profile.

## Authorization boundary

Treat each of these as a fresh authorization boundary:

- `snapshot refresh`
- Mac mini access
- production database reads or transfer
- production Profile operations
- deployment, migration, or production service changes

After explicit refresh authorization, follow `docs/DATA_PROFILES.md` and still
use `./scripts/data-profile.sh ... snapshot refresh`; do not copy, edit, delete,
or inspect database contents through a second path. Refresh success must leave
the active Profile unchanged until the user explicitly requests a switch.
The local command cannot select `production`.

## Failure handling

Keep the previous Profile running. Distinguish:

- refresh rejected or failed;
- switch failed and rolled back;
- process running but `/ready` false;
- status unmanaged or unavailable.

Show the error summary and the last confirmed status. Do not expose absolute
database paths, credentials, transfer configuration, private data, or raw
database contents.
