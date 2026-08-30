---
layout: default
title: Project lifecycle
---

# Omurga project lifecycle v1

The project lifecycle manages a project on a local supported Linux host. Remote
execution reuses the same reconciliation contract through SSH host profiles.

## Deploy sequence

`omurga project deploy` performs these operations in order:

1. Load, merge, and validate the project manifest.
2. Verify Docker and, when required, Caddy and systemd.
3. Verify required runtime secret files and restrictive permissions.
4. Allocate stable gateway ports in SQLite.
5. Generate Compose and Caddy artifacts for the deployment paths.
6. Create project data directories with restrictive permissions.
7. Preserve the previous Compose artifact and atomically write the new one.
8. Run `docker compose config --quiet`.
9. Run `docker compose up --detach --remove-orphans --wait` with a 120-second
   health timeout.
10. Reconcile the project Caddy snippet after container health succeeds.
11. Ensure the base Caddyfile imports Omurga project snippets.
12. Validate the complete Caddy configuration and reload the Caddy service.
13. Store the successful deployment revision and paths in SQLite.

Caddy receives only loopback upstreams. Regular deployments reuse stable ports
allocated from `20000-29999`; project containers are not published on public
host interfaces.

When an already-running project has only stateless services, no dependencies,
and no bind-mounted volumes, Omurga uses a blue-green deployment path. The new
Compose project is started in the inactive `a` or `b` slot with ephemeral
loopback ports, health-checked, and connected to Caddy by an atomic reload.
Only after the gateway points at the replacement does Omurga stop the old
slot. The active slot is recorded in SQLite, so status, logs, restart, stop,
rollback, and delete continue to target the correct containers.

Projects with PostgreSQL, Redis, other dependencies, or bind-mounted volumes
use the regular in-place deployment path. Running two versions against shared
persistent state would be unsafe, so those projects still receive health
checks and automatic artifact/container rollback.

## Failure and rollback behavior

The active Caddy configuration is not changed before the new containers become
healthy. If Compose validation fails, the previous Compose artifact is restored.
If container startup or health checks fail, Omurga restores and reconciles the
previous Compose deployment. A failed first deployment is brought down and its
new artifact is removed.

If Caddy validation or reload fails, the previous project snippet and base
Caddyfile are restored. Omurga then restores the previous Compose deployment.
Previous artifacts are retained as `compose.previous.yaml` and
`<project>-<environment>.caddy.previous` for explicit rollback.

During a blue-green deployment, a failed replacement is stopped and removed;
the old slot and its Caddy route remain active. If deployment state cannot be
stored after the gateway switch, Omurga restores the previous route and removes
the replacement before returning the error.

`project rollback` validates and health-checks the previous Compose artifact
before committing the operation. Caddy follows the same validation and reload
rules as deploy. Current and previous artifacts are swapped only as a successful
unit, which makes a second rollback act as a roll-forward. A failed rollback
restores both artifact slots and reconciles the containers that were active
before the attempt.

## Dry-run behavior

`--dry-run` validates and renders the desired configuration in memory. It
reports paths, commands, gateway ports, and required secret names. It does not:

- require root privileges;
- create directories or files;
- create or migrate SQLite state;
- reserve gateway ports;
- inspect secret values;
- execute Docker, Caddy, or systemd commands.

When a state database already exists, dry-run opens it read-only so planned
ports account for current host allocations.

## Status and controls

`project status` reads the deployment record and calls Docker Compose for live
container state. JSON output includes normalized container objects.

`project restart` and `project stop` operate on the generated Compose project
and update the SQLite deployment status. Both support dry-run planning. Runtime
mutations require root privileges in the current local-host implementation.

## Logs

`project logs` streams Docker Compose output directly, including follow mode and
context cancellation. It supports `--tail`, `--since`, `--timestamps`, and one
or more `--service` filters. Service filters must reference a declared project
service or dependency. Structured JSON output is available for a dry-run log
plan; live log lines remain the original container output.

## Project deletion

`project delete` removes project containers, the active Caddy route, generated
artifacts, runtime secrets, the deployment record, and gateway port assignments.
The manifest source is not deleted.

Persistent project data under the deployment `data` directory is preserved by
default. Permanent deletion requires `--purge-data --yes`. The purge target is
resolved and verified as a child of the exact project deployment directory
before recursive removal. Deletion can be safely retried after a partially
completed attempt.

The Caddy route is removed only after containers are stopped. The resulting
complete Caddy configuration is validated before reload; validation failure
restores the route.
