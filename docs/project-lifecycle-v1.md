# Omurga project lifecycle v1

The initial project lifecycle manages a project on the local Ubuntu host. Remote
host execution will reuse this reconciliation contract after SSH host support is
implemented.

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

Caddy receives only loopback upstreams allocated from `20000-29999`. Project
containers are not published on public host interfaces.

## Failure and rollback behavior

The active Caddy configuration is not changed before the new containers become
healthy. If Compose validation fails, the previous Compose artifact is restored.
If container startup or health checks fail, Omurga restores and reconciles the
previous Compose deployment. A failed first deployment is brought down and its
new artifact is removed.

If Caddy validation or reload fails, the previous project snippet and base
Caddyfile are restored. Omurga then restores the previous Compose deployment.
Previous artifacts are retained as `compose.previous.yaml` and
`<project>-<environment>.caddy.previous` for the explicit rollback command that
will be added next.

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
