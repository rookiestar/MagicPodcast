---
name: magicpodcast-remote-deployment
description: Safely deploy, start, restart, or verify MagicPodcast on its designated Mac mini through a Codex remote workspace or an explicitly approved SSH path. Use for Mac mini deployment, production builds, Cloudflare named Tunnel setup, Nginx or launchd service work, remote port diagnosis, rollback, and live HTTPS/Access acceptance. Enforce target-host proof, user authorization gates, loopback-only listeners, secret isolation, and runtime verification.
---

# MagicPodcast Remote Deployment

Deploy only to the designated Mac mini and prove the live security boundary. Treat the local operator Mac, a local checkout, and a Codex App connection label as insufficient evidence that commands are running on the target.

## Read First

1. Read `AGENTS.md`, `README.md`, `docs/README.md`, `docs/DEPLOYMENT.md`, and `docs/ENV_SETUP.md` from the remote checkout.
2. Read the full body, comments, and labels of the active GitHub Issue. Read Issue #1 when its PRD requirements are relevant. Do not edit or close the PRD.
3. Read task-specific source, tests, scripts, Nginx configuration, Cloudflare configuration metadata, and launchd definitions before changing them.
4. Inspect `git status --short` and preserve all unrelated or pre-existing changes.

## Prove the Execution Target

Run the bundled preflight before any remote mutation:

```bash
.agents/skills/magicpodcast-remote-deployment/scripts/preflight.sh \
  --expected-host '<confirmed-mac-mini-hostname>' \
  --repo-root '<absolute-remote-repository-path>'
```

Obtain the expected hostname from the user or previously verified remote evidence. Never guess it from the connection label `Mac Mini`.

The preflight must prove all of the following:

- the current host matches the confirmed Mac mini;
- the absolute Git root is the intended `rookiestar/MagicPodcast` checkout;
- current port listeners and their owners are visible;
- no listener on `3000`, `8080`, or `8088` is bound beyond loopback;
- required tools and existing Tunnel/launchd metadata are reported without printing secrets.

Stop if target identity, repository identity, or port safety is unproven. A Codex App remote connection and a shell SSH alias are separate mechanisms; do not assume one configures the other.

## Preserve the Source Boundary

- Work directly in the verified remote workspace when the task is attached to the Mac mini checkout.
- Do not deploy from the local mirror merely because its files look current.
- Do not copy an entire dirty worktree to the remote host.
- If source must move between machines, state the exact transfer method and request any required `git add`, commit, push, pull, rsync, or file-transfer authorization first.
- Never transfer databases, `.env` files, Cloudflare credentials, Tunnel credential JSON, `cert.pem`, logs, backups, or other secrets with source code.

## Authorization Gates

Read-only inspection and non-writing targeted checks are allowed. Require explicit user authorization before each scoped operation involving:

- Cloudflare Access policies, DNS, HTTPS/HSTS, Tunnel creation/routing, or session revocation;
- production builds, service start/stop/restart, Nginx reload, launchd changes, or port/router changes;
- dependency installation, removal, or upgrades;
- real database migration, restore, import, synchronization, maintenance, manual SQL, or backup replacement;
- real LLM calls or workflows that may trigger them;
- `git add`, commit, push, pull with integration risk, branch mutation, or Pull Request creation.

Reuse an explicit authorization already given for the same operation and target. Do not expand it to adjacent services, unrelated containers, credentials, or data.

## Remote Deployment Workflow

1. Run preflight and record the remote hostname, repository root, branch, worktree state, tool availability, port owners, and current service state.
2. Inspect the exact production command before running it. Detect implicit dependency installation, builds, migrations, syncs, or cleanup and gate them separately.
3. Run `magicpodcast-code-change-verification` for source or script changes. Add narrower checks for deployment scripts and configuration.
4. Resolve port conflicts by identifying the owning process. Never stop, reconfigure, or relaunch an unrelated application without explicit authorization.
5. Confirm production configuration keeps the backend and frontend on numeric loopback addresses. Confirm Nginx, if used, listens only on `127.0.0.1:8088`.
6. Build and start only after the corresponding authorization. Verify process ownership after each service starts; do not accept a port number alone as proof.
7. Configure and start the named Tunnel only after the origin is healthy and Access/HTTPS protections are ready.
8. Run local, Tunnel, and public acceptance checks. Roll back immediately when a security invariant fails.

## Cloudflare Named Tunnel Rules

- Use only the named Tunnel `magicpodcast-prod`. Never use a Quick Tunnel.
- Authenticate `cloudflared` on the Mac mini itself. Never copy `cert.pem` or Tunnel credentials from the operator Mac.
- If the Tunnel already exists, inspect and reuse it instead of creating a duplicate.
- Route only `rookiestar.cn` to `http://127.0.0.1:8088` and end ingress with `http_status:404`.
- Keep `$HOME/.cloudflared` owner-only and credential/config files mode `600`.
- Do not print certificate contents, credential JSON, tokens, owner email, callback URLs, or login redirects in logs or reports.
- Do not install persistent launchd startup before the active Issue allows it and the user authorizes it.

## Runtime Acceptance

Collect current runtime evidence for every applicable item:

- `3000`, `8080`, and `8088` listen only on numeric loopback and belong to the expected MagicPodcast/Nginx processes;
- local frontend, backend health, and Nginx origin respond as intended;
- `cloudflared tunnel info magicpodcast-prod` reports a live connector on the Mac mini;
- HTTP permanently redirects to HTTPS;
- HTTPS includes `Strict-Transport-Security: max-age=31536000`, without subdomain inclusion or preload;
- unauthenticated page, API, image, framework asset, and health requests all enter Cloudflare Access;
- the allowed Google identity completes Google login plus the configured Cloudflare MFA/Passkey step;
- a disallowed identity is rejected and a revoked Access session loses access;
- another LAN device cannot directly reach `3000`, `8080`, or `8088`, and the router has no application-port forwarding.

Do not replace runtime evidence with source inspection. Do not trigger writes, syncs, workflows, or LLM calls during read-only acceptance.

## Rollback

- On an Access, HTTPS, origin, or Tunnel acceptance failure, stop and disable the Tunnel first.
- Restore only the smallest previously recorded configuration item needed to recover.
- Keep recovery access limited to the Mac mini console or a temporary SSH forward.
- Never recover by disabling Access, enabling Basic Auth, creating a Quick Tunnel, exposing a LAN port, or adding a bypass policy.
- Treat any rollback that republishes a previous public origin as a new externally visible change requiring authorization.

## Hard Stops

- Stop when the shell is not proven to be the target Mac mini.
- Stop when the remote repository or branch is ambiguous.
- Stop before a production command that would install missing dependencies without explicit authorization.
- Stop before touching a real database and invoke `magicpodcast-database-change-guard`.
- Never introduce application accounts, passwords, registration, roles, password recovery, API tokens, or a permanent LAN bypass.
- Never kill an unknown process merely because it owns a desired port.
- Never report a build, restart, Tunnel connection, Access rule, or public behavior as complete without current evidence.

## Report

Report concisely in Chinese:

- 改了什么
- 验证了什么
- 未运行或失败了什么
- 剩余风险
- 未触碰的文件、服务和数据

Include the verified remote hostname and repository root. Distinguish local checks, remote checks, and public checks.
