---
layout: default
title: Project manifest specification
---

# Project manifest v1

## Files

A project contains a base manifest and optional environment overlays:

```text
demo/
|-- omurga.yaml
`-- environments/
    `-- production.yaml
```

Maps are merged recursively, scalars are replaced, and lists are replaced as a
whole. An environment overlay cannot change `version` or `name`. Unknown fields
are rejected.

## Services

Each service requires an image and can define pull and restart policies,
commands, internal ports, environment values, secret files, persistent bind
mounts, resource limits, logging limits, and a health check.

Omurga maps `if-not-present` to the Compose `missing` pull policy. Restart
defaults to `unless-stopped`. Resource values are rendered with service-level
`cpus`, `mem_limit`, and `pids_limit` attributes.

## Gateway

Every gateway route references a declared service and one of its exposed ports.
The generated Compose port uses long syntax with `host_ip: 127.0.0.1`. Caddy
proxies the public domain to that loopback port. A route with `https: false`
uses an explicit `http://` Caddy site address; HTTPS is otherwise the default.
For HTTPS routes, Caddy obtains and renews publicly trusted certificates
automatically when the domain is publicly resolvable and ports 80 and 443 reach
the host. Set `gateway.email` to identify the ACME account:

```yaml
gateway:
  email: ops@example.com
  routes:
    - domain: app.example.com
      service: app
      port: 80
      https: true
```

Local names such as `.localhost`, `.local`, and `.internal` do not qualify for
public certificates. Use `https: false` for local HTTP-only testing.

Preview ports are deterministic values in the `20000-29999` range. Preview
ports are intended for rendering only and do not create operational state.
Deployment uses SQLite to preserve stable assignments and avoid collisions
across projects. Dry-run planning reads existing assignments without writing.

## Secrets

Top-level Compose secrets use files under:

```text
/run/omurga/secrets/<project>/<environment>/<secret>
```

A service receives access only when the secret is explicitly listed in that
service. Absolute secret targets are supported. UID, GID, and mode are applied
to the runtime file by Omurga's secret materialization layer rather than emitted
as Compose secret attributes, because Docker Compose ignores those attributes
for file-backed secrets.

## Project dependencies

Project-scoped PostgreSQL and Redis dependencies are rendered as Compose
services with persistent bind mounts and health checks. PostgreSQL requires a
database, user, and `passwordSecret`. Redis supports `aof`, `rdb`, and `none`
persistence modes, plus memory and eviction policy settings.

Shared dependency instances are part of the v1 design but are not rendered yet.
The renderer returns an explicit error instead of silently treating a shared
dependency as project-scoped.

## Commands

```bash
omurga project create demo
omurga project validate ./demo
omurga project validate ./demo --env production
omurga project render ./demo --env production
omurga project render ./demo --env production --kind caddy
```
