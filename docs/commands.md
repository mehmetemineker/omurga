---
layout: default
title: Command guide
description: A scenario-based guide to every Omurga command.
---

# Command guide

Use this page to choose the right command. Every command has its own page with
the purpose, a practical scenario, a copy-and-paste example, and safety notes.

## Before you start

Most commands operate on the local host unless `--host <name>` is supplied.
Host-changing commands require `sudo`. Project paths point to a directory
containing `omurga.yaml`; use `.` when you are inside that directory.

```bash
omurga --help
omurga project --help
omurga project deploy --help
```

Global options:

| Option | Use it when you need to |
| --- | --- |
| `--host <name>` | Run the command on a configured SSH host. |
| `--env <name>` | Load a project environment overlay. |
| `--dry-run` | Preview a host-changing operation without applying it. |
| `--json` | Consume the result from a script or CI job. |
| `--quiet`, `-q` | Use only the exit code. |
| `--yes`, `-y` | Confirm an operation that explicitly requires it. |
| `--progress <mode>` | Choose `auto`, `tty`, `plain`, or `off`. |

`--json`, `--quiet`, and `--dry-run` disable progress output so command output
remains safe for scripts.

## Choose a category

### Start and maintain a host

- [Host overview]({{ '/commands/host/' | relative_url }}) — detect, initialize, install components, and update Debian or Ubuntu.
- [Doctor]({{ '/commands/doctor/' | relative_url }}) — check the complete host health.

### Create and operate projects

- [Project overview]({{ '/commands/project/' | relative_url }}) — create, validate, render, deploy, repair, inspect, and remove projects.
- [Project lifecycle]({{ '/commands/project/lifecycle/' | relative_url }}) — deploy, repair, status, restart, stop, rollback, and delete.
- [Project service access]({{ '/commands/project/access/' | relative_url }}) — execute commands, open a shell, and read logs.
- [Project inspection]({{ '/commands/project/inspection/' | relative_url }}) — inspect configuration and compare revisions.
- [Environment overview]({{ '/commands/environment/' | relative_url }}) — manage non-secret environment overlays.
- [Secret overview]({{ '/commands/secret/' | relative_url }}) — store encrypted project secrets.
- [Gateway overview]({{ '/commands/gateway/' | relative_url }}) — inspect and reload Caddy routes.

### Operate data and backups

- [PostgreSQL]({{ '/commands/postgres/' | relative_url }}) — inspect instances, manage databases and users, and perform dumps/restores.
- [Redis]({{ '/commands/redis/' | relative_url }}) — inspect instances, open Redis CLI, back up, and flush data.
- [Backup overview]({{ '/commands/backup/' | relative_url }}) — create, restore, prune, and schedule Restic backups.

### Monitor and notify

- [Monitoring]({{ '/commands/monitoring/' | relative_url }}) — install and operate Prometheus, Grafana, Node Exporter, and cAdvisor.
- [Alerts]({{ '/commands/alert/' | relative_url }}) — check, test, schedule, and inspect Telegram/email alerts.
- [Support bundle]({{ '/commands/support/' | relative_url }}) — create a safe diagnostic archive.

### Manage infrastructure integrations

- [Shared services]({{ '/commands/service/' | relative_url }}) — install and operate catalog services.
- [Registries]({{ '/commands/registry/' | relative_url }}) — configure Docker registry profiles.
- [Remote hosts]({{ '/commands/remote/' | relative_url }}) — manage SSH host profiles.
- [Webhooks]({{ '/commands/webhook/' | relative_url }}) — configure signed image deployment webhooks.

### Read-only and utility commands

- [Version]({{ '/commands/version/' | relative_url }}) — show the installed build information.

## Common scenarios

### “I have a new Raspberry Pi”

```bash
omurga host detect
sudo omurga --dry-run host init
sudo omurga host init
sudo omurga doctor
```

Continue with the [host guide]({{ '/commands/host/' | relative_url }}).

### “I changed the image or environment and want to deploy it”

```bash
omurga project validate ./demo --env production
omurga project diff ./demo --env production
sudo omurga --dry-run project deploy ./demo --env production
sudo omurga project deploy ./demo --env production
sudo omurga project status ./demo --env production
```

Continue with [project lifecycle]({{ '/commands/project/lifecycle/' | relative_url }}).

### “The application is unhealthy”

```bash
sudo omurga project status ./demo --env production
sudo omurga project logs ./demo --env production --service app --tail 200
sudo omurga project inspect ./demo --env production
sudo omurga project repair ./demo --env production
sudo omurga support bundle
```

Continue with [project access]({{ '/commands/project/access/' | relative_url }}) and
[support bundle]({{ '/commands/support/' | relative_url }}).

### “I need to investigate a host problem”

```bash
sudo omurga doctor --json > doctor.json
sudo omurga support bundle --output /tmp/omurga-support.tar.gz
```

The support archive does not include secret values, environment values,
configuration files, or logs.
