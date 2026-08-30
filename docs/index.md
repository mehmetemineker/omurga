---
layout: default
title: Omurga Documentation
description: The practical guide to provisioning Linux hosts and operating Docker projects with Omurga.
---

<div class="hero">
<p class="kicker">Linux operations, made repeatable</p>

# Run your server with a clear spine.

<p class="lead">Omurga is a declarative CLI for Debian and Ubuntu hosts. It provisions the host, runs Docker Compose projects, configures Caddy, manages secrets and data services, and gives you backups, alerts, and monitoring in one workflow.</p>

<div class="hero-actions">
<a class="button" href="{{ '/getting-started/' | relative_url }}">Start in five minutes</a>
<a class="button secondary" href="{{ '/commands/' | relative_url }}">Browse all commands</a>
</div>
</div>

## What Omurga manages

<div class="cards">
<div class="card"><h3>Hosts</h3><p>Detect Debian or Ubuntu, install Docker, Caddy, Restic, automatic security updates, UFW, and Fail2ban, update packages, and run health checks.</p></div>
<div class="card"><h3>Projects</h3><p>Define services in YAML, merge environments, deploy safely, configure gateways, and roll back.</p></div>
<div class="card"><h3>Operations</h3><p>Back up data, send Telegram or email alerts, manage registries, and inspect Prometheus metrics in Grafana.</p></div>
</div>

## The shortest path to a running project

```bash
# On a Debian or Ubuntu host
sudo omurga host init

# Create a project locally
omurga project create demo
omurga project validate ./demo
sudo omurga project deploy ./demo
sudo omurga project status ./demo
```

For a Raspberry Pi, download the `arm64` release asset. For Intel or AMD
servers, download the `amd64` asset. The [Getting started guide](getting-started.md)
walks through installation and the first deployment.

## Core concepts

### A manifest is the source of truth

Every project has an `omurga.yaml` file. It declares services, images, exposed
ports, health checks, dependencies, gateway routes, backups, and alert events.
Environment overlays change non-secret values without duplicating the base
manifest. Secret values live in an encrypted age store and are materialized
only while a deployment needs them.

### The host is intentionally boring

Docker Compose runs the application containers. Caddy is the public HTTP and
HTTPS entry point. Application ports bind to loopback, so containers are not
accidentally exposed directly. Omurga records deployment state and preserves
the previous healthy artifacts for rollback.

### Safe by default

Use `--dry-run` to inspect a plan before a host-changing command. Persistent
project, database, Redis, shared-service, and monitoring data is preserved by
default. Destructive operations require an explicit `--yes` together with a
purge flag.

## Documentation map

- [Getting started](getting-started.md) — installation, host initialization, first project, and Pi workflow.
- [Projects and environments](projects.md) — manifests, overlays, secrets, deployment, Caddy, PostgreSQL, and Redis.
- [Operations](operations.md) — backups, alerts, monitoring, registries, shared services, and remote hosts.
- [Command reference](commands.md) — every command, argument, option, and common flag.
- [Troubleshooting](troubleshooting.md) — diagnostics for Docker, Caddy, deployments, backups, and alerts.

The versioned technical notes remain available for implementation details:
[architecture](architecture-v1.md), [manifest specification](project-manifest-v1.md),
[deployment lifecycle](project-lifecycle-v1.md), and [operations design](operations-v1.md).
