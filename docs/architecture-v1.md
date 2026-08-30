---
layout: default
title: Architecture
---

# Omurga v1 architecture

## Purpose

Omurga is a declarative CLI that provisions supported Linux hosts and manages
the day-to-day operation of Docker Compose projects. Ubuntu and Debian are the
first supported distributions. A manifest defines the desired state; Omurga
inspects the current state and reaches the desired state through safe,
repeatable operations.

## Fixed decisions

- The application is distributed as a single Go binary.
- Docker Engine and Docker Compose provide the container runtime.
- Docker uses the `local` log driver with Omurga-managed size and file-count rotation defaults.
- Daily security updates use `unattended-upgrades`; automatic reboot is disabled and runtime-service packages are excluded.
- The current Ubuntu and Debian providers run Caddy as a systemd service.
- Projects are defined by a `version: 1` YAML manifest.
- Operational state is stored in SQLite; the manifest is the source of truth.
- Secrets are stored in files encrypted with `age`.
- Decrypted secrets are stored under `/run/omurga` and mounted read-only.
- The current Ubuntu and Debian providers schedule jobs as systemd timers.
- Restic is the backup engine; PostgreSQL uses `pg_dump` and Redis uses snapshots.
- The first remote backup targets are S3-compatible storage and SFTP.
- The first alert channels are Telegram and SMTP email.
- UFW is installed as an optional host security component with deny-incoming,
  allow-outgoing, SSH, HTTP, and HTTPS rules.
- Fail2ban is installed as an optional host security component with an Omurga-managed SSH jail.
- Host monitoring checks CPU load, memory, disk capacity, failed services,
  managed container health, and Caddy certificate expiry through a persistent
  systemd timer.
- Optional monitoring runs Prometheus, Grafana, Node Exporter, and cAdvisor as
  a dedicated managed Docker stack.
- Remote hosts are managed over SSH without a continuously running Omurga daemon.
- Image deployment webhooks accept only signed CI callbacks containing immutable image digests.

## Host directories

```text
/usr/local/bin/omurga
/etc/omurga/config.yaml
/etc/omurga/projects/<project>/omurga.yaml
/etc/omurga/projects/<project>/environments/<environment>.yaml
/etc/omurga/secrets/<project>/<environment>.age
/etc/omurga/keys/identity.agekey
/etc/caddy/omurga-<project>-<environment>.caddy
/var/lib/omurga/state.db
/var/lib/omurga/projects/<project>/<environment>/compose.yaml
/var/lib/omurga/projects/<project>/<environment>/data/
/var/lib/omurga/services/monitoring/data/
/etc/omurga/monitoring/grafana-admin-password
/var/backups/omurga/staging/
/var/log/omurga/audit.jsonl
/run/omurga/secrets/<project>/<environment>/
```

## Manifest merge contract

The base file is `omurga.yaml`. Environment-specific changes are stored in
`environments/<environment>.yaml`.

- Maps are merged recursively.
- Scalars are replaced by the value in the environment file.
- Lists are not appended; the environment list replaces the base list entirely.
- Environment files cannot override `version` or `name`.
- Unknown fields are validation errors.
- No host changes are made before the manifest is validated.

## Gateway model

Caddy runs on the host. A container port exposed through the gateway is
published only on `127.0.0.1`, using a stable port allocated by Omurga from the
`20000-29999` range. Port assignments are stored in SQLite. Caddy proxies to
this localhost port, and project containers are not exposed directly to the
internet.

A deployment validates the Compose and Caddy configurations, pulls images,
checks container health, and atomically reloads Caddy. Stateless gateway
projects are deployed blue-green into alternating Compose slots with ephemeral
loopback ports; the old slot is stopped only after the new route is active.
Projects with persistent volumes or dependencies use the in-place path. The
existing gateway route is preserved when a health check fails, and failed
blue-green replacements are removed automatically.

## Secret model

A secret value is read from a file or standard input and is never passed as a
command argument. The persistent store is encrypted with `age`. During a
deployment, values are decrypted under `/run/omurga/secrets`, the UID, GID, and
file mode from the manifest are applied, and each file is mounted under
`/run/secrets` as a Docker Compose secret. Logs and normal configuration output
never contain secret values.

## Data services

The default mode for PostgreSQL and Redis is an isolated instance per project.
Shared instances are accessed through an internal Docker network named
`omurga-shared`. PostgreSQL backups use `pg_dump` instead of copying a live data
directory. Redis persistence accepts `aof`, `rdb`, and `none`.

## Backup and scheduling

A Restic snapshot includes database dumps, Redis snapshots, project volumes,
manifests, the encrypted secret store, and an Omurga state backup. A backup is
successful only after it reaches the remote destination. Default retention is
7 daily, 4 weekly, and 6 monthly snapshots. Scheduling uses systemd timers.

## Doctor exit codes

- `0`: healthy
- `1`: warnings present
- `2`: critical failure present

Doctor checks the operating system, reboot requirements, disk and inode usage,
Docker, Caddy, Caddy's service-account access to its configuration, container
health, secret file permissions, state integrity, systemd backup timers, disk
capacity, failed services, and certificate expiry.
It provides both human-readable and JSON output.

## Security boundaries

- Operations that modify a host require root or sudo.
- All configuration writes are atomic and validated first.
- Changes are written to a structured audit log.
- `--dry-run` shows the planned changes.
- Project removal preserves volumes and databases by default.
- Persistent data deletion requires an explicit `--purge-data` flag and confirmation.
- Restore operations create a safety backup by default.

## Delivery order

1. CLI, configuration, output, and manifest validation
2. Linux platform providers, APT, Docker, Caddy, and basic doctor checks
3. Compose generation and persistent gateway port allocation
4. The project lifecycle and the encrypted secret store
5. PostgreSQL and Redis operations
6. Restic backups, systemd timers, and alert channels
7. Multi-host management over SSH

## Implementation status

The v1 implementation includes:

- strict manifest loading, environment merging, and validation
- idempotent creation of Omurga host directories and the initial host config
- Ubuntu 22.04, 24.04, and 26.04 detection and provisioning
- Debian 11, 12, and 13 detection and provisioning
- extensible distribution, package-manager, and service-manager contracts
- safe and full APT upgrade modes with dry-run support
- idempotent Docker Engine and Compose installation from Docker's official APT repository
- idempotent Caddy installation from Caddy's official stable APT repository
- doctor checks for the operating system, privileges, managed directories, APT,
  Docker, Docker Compose, Docker and Caddy systemd services, reboot requirements,
  and disk usage
- structured JSON output and doctor exit codes
- project scaffold generation and strict environment overlays
- deterministic Docker Compose and Caddy artifact rendering
- loopback-only gateway port publishing
- a versioned SQLite state database with read-only dry-run access
- stable, transaction-safe gateway port allocation across all projects
- project deploy, status, logs, restart, stop, rollback, and delete operations
- Compose health waiting and pre-reload Caddy validation
- automatic artifact and runtime rollback on deployment failure
- project-scoped PostgreSQL and Redis Compose services
- encrypted age secret stores and deployment-time materialization
- project and environment inspection and non-secret environment editing
- project-scoped PostgreSQL and Redis operational commands
- shared Docker services and registry profiles
- Restic backup, restore, retention, integrity, and systemd timer operations
- Telegram and TLS SMTP alert delivery
- SSH host profiles and daemonless remote command forwarding
- expanded doctor checks for SQLite, secret permissions, container health,
  filesystem capacity, inodes, gateway configuration, Restic, and backup timers

The installer refuses to remove conflicting distribution Docker packages unless
`--replace-conflicting-docker` is explicitly provided. It does not use Docker's
convenience script. Repository files and public signing keys are written
atomically and package services are enabled through systemd.

The installation contracts follow the official
[Docker Engine on Ubuntu](https://docs.docker.com/engine/install/ubuntu/),
[Docker Engine on Debian](https://docs.docker.com/engine/install/debian/), and
[Caddy Debian/Ubuntu package](https://caddyserver.com/docs/install#debian-ubuntu-raspbian)
documentation.

Project rendering generates artifacts without changing SQLite, Docker, or Caddy
runtime state. Destructive data removal, Redis flushes, database restores, and
Restic pruning require explicit flags. Runtime integration must still be
validated on the target supported Linux host because container images, DNS,
TLS, remote repositories, SMTP servers, and Telegram credentials are
deployment-specific.
