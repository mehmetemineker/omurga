---
layout: default
title: State model
---

# Omurga operational state v1

Omurga stores host-local operational state in `/var/lib/omurga/state.db`. The
project manifest remains the source of truth; SQLite contains runtime decisions
that must remain stable between commands and cannot be represented safely by a
standalone render preview.

## Database lifecycle

- The database uses SQLite through a pure Go driver and does not require CGO.
- Schema changes are controlled by `PRAGMA user_version`.
- The current schema version is `2`.
- A database created by a newer Omurga schema is rejected.
- The database file mode is `0600`.
- WAL mode and a five-second busy timeout are enabled for normal access.
- Mutating allocation operations use `BEGIN IMMEDIATE` transactions.
- Dry-run access opens an existing database in read-only mode and never creates
  a database or parent directory.

## Gateway port assignments

Gateway ports are allocated from `20000-29999`. Each assignment is identified
by project, environment, service, and container port. The host port has a global
unique constraint, so two managed routes cannot receive the same loopback port.

The first candidate is deterministic. When that port is already allocated,
Omurga probes the managed range until it finds a free port. The assignment is
then stored transactionally and reused by later deployments.

The `default` environment key represents a project without an environment
overlay. Removing a route does not immediately release its assignment. This
preserves stability across temporary manifest changes; explicit project cleanup
will own release behavior in the lifecycle implementation.

## Deployment records

Schema version 2 stores one deployment record for each project and environment.
The record includes its status, artifact revision, manifest path, Compose path,
optional Caddy path, update time, and last operational error. Successful deploy,
restart, and stop operations update this record.

The artifact revision is a SHA-256 digest of the generated Compose and Caddy
content. It identifies generated runtime configuration without storing secret
values.

Project deletion removes the deployment record and its gateway port assignments
in one immediate transaction. Persistent project data is filesystem state and
is not stored in or removed through SQLite.

## Render and dry-run behavior

`omurga project render` remains a standalone, non-mutating operation and uses
deterministic preview assignments. Deployment planning resolves ports against
the state database. With `--dry-run`, existing assignments and collisions are
read without persisting newly planned ports.
