---
layout: default
title: Command reference
description: Complete reference for every Omurga command and global option.
---

# Command reference

The examples below assume Omurga is installed as `/usr/bin/omurga` or is
available on `PATH`. Commands that change a Linux host or Docker state usually
require `sudo`. Commands that only read or render project files do not.

## Global options

Global options can be placed after `omurga` and are inherited by subcommands.

| Option | Default | Purpose |
| --- | --- | --- |
| `--host <name>` | `local` | Run against a configured remote host profile. |
| `--env <name>` | empty | Select a project environment overlay. |
| `--json` | false | Write machine-readable JSON. |
| `--quiet`, `-q` | false | Suppress successful output. |
| `--dry-run` | false | Show the plan without changing state. |
| `--yes`, `-y` | false | Accept operations that explicitly require confirmation. |
| `--progress <mode>` | `auto` | Select `auto`, `tty`, `plain`, or `off`. |

Examples:

```bash
omurga --json project status ./blog
sudo omurga --dry-run host update
sudo omurga --progress plain project deploy ./blog
sudo omurga --quiet doctor
```

`--json`, `--quiet`, and `--dry-run` disable progress output automatically.
This keeps command output safe for scripts and CI logs.

## Top-level commands

### `omurga version`

Print the binary version, Git commit, and build date.

```bash
omurga version
omurga version --json
```

### `omurga doctor`

Run health checks for the active host. A non-zero exit code indicates warnings
or critical checks according to the doctor result.

```bash
sudo omurga doctor
sudo omurga doctor --json
```

## Host commands

### `omurga host detect`

Detect the distribution and the provider selected by Omurga.

```bash
omurga host detect
omurga host detect --json
```

### `omurga host init`

Create managed directories and install Docker, Caddy, Restic, UFW, and Fail2ban.
UFW allows SSH, HTTP, and HTTPS by default. The command is idempotent and safe
to run again after an interrupted setup.

Options:

- `--skip-docker` — do not install Docker.
- `--skip-caddy` — do not install Caddy.
- `--skip-restic` — do not install Restic.
- `--skip-ufw` — do not install or configure UFW.
- `--skip-fail2ban` — do not install Fail2ban.
- `--ssh-port <port>` — SSH TCP port to allow in UFW (default: `22`).
- `--replace-conflicting-docker` — remove conflicting distribution Docker packages before installing Docker CE.

```bash
sudo omurga --dry-run host init
sudo omurga host init
sudo omurga host init --skip-restic
sudo omurga host init --replace-conflicting-docker
```

### `omurga host install <component>`

Install or repair one component. `<component>` is `docker`, `caddy`, `restic`,
`ufw`, `fail2ban`, or `all`.

Docker installation also configures the default container log rotation policy:
the `local` driver, `10m` maximum file size, and three rotated files.

Option:

- `--replace-conflicting-docker` — allow replacement of conflicting Docker packages.
- `--ssh-port <port>` — SSH TCP port to allow in UFW (default: `22`).

```bash
sudo omurga --dry-run host install docker
sudo omurga host install docker
sudo omurga host install caddy
sudo omurga host install restic
sudo omurga host install ufw --ssh-port 22
sudo omurga host install fail2ban
sudo omurga host install all
```

### `omurga host update`

Refresh APT indexes and upgrade installed packages.

Option:

- `--full` — use the provider’s full distribution upgrade mode.

```bash
sudo omurga --dry-run host update
sudo omurga host update
sudo omurga host update --full
```

### `omurga host status` and `omurga host doctor`

Both commands run the host doctor checks and are provided as convenient host
subcommands:

```bash
sudo omurga host status
sudo omurga host doctor
```

### Remote host profile commands

These commands manage local SSH profiles. They do not contact the remote host:

```bash
omurga host add <name> <address> [options]
omurga host list
omurga host show <name>
omurga host remove <name>
```

Options for `host add`:

- `--user <user>` — SSH user.
- `--port <port>` — SSH port, default `22`.
- `--identity <path>` — SSH private key.
- `--omurga-path <path>` — binary path on the remote host, default `omurga`.
- `--sudo=false` — do not invoke remote Omurga through non-interactive sudo.

```bash
omurga host add pi 192.168.0.50 \
  --user mehmet --identity ~/.ssh/id_ed25519
omurga host list
omurga host show pi
omurga host remove pi
```

Once a profile exists, pass `--host <name>` to any supported operation:

```bash
omurga --host pi doctor
omurga --host pi --dry-run host update
omurga --host pi monitoring status
```

The target host must already have the Omurga binary installed. Remote commands
use SSH; followed project logs and interactive database shells allocate a TTY.

## Project commands

### `omurga project create <name>`

Create a project scaffold containing `omurga.yaml` and an `environments`
directory.

Option:

- `--directory <path>` — parent directory, default `.`.

```bash
omurga project create blog
omurga project create blog --directory ~/omurga-lab
```

### `omurga project validate [path]`

Validate the base manifest and, when selected, its merged environment overlay.
The path defaults to the current directory.

```bash
omurga project validate ./blog
omurga --env production project validate ./blog
omurga --json project validate ./blog
```

### `omurga project show [path]`

Print the resolved project manifest after loading the selected environment.

```bash
omurga --env production project show ./blog
```

### `omurga project render [path]`

Render generated configuration without deploying it.

Options:

- `--kind <kind>` — `compose` or `caddy`, default `compose`.
- `--output`, `-o <path>` — write to a file instead of stdout; default `-`.

```bash
omurga --env production project render ./blog --kind compose
omurga --env production project render ./blog --kind caddy
omurga project render ./blog --kind compose --output /tmp/blog-compose.yaml
```

### `omurga project deploy [path]`

Reconcile the project to its desired state. Deployment validates configuration,
pulls images, starts services, waits for health, validates Caddy, and reloads
the gateway. A failed deployment automatically restores the previous healthy
state when one exists.

```bash
sudo omurga --dry-run --env production project deploy ./blog
sudo omurga --env production project deploy ./blog
```

### `omurga project status [path]`

Show the recorded deployment revision and live Compose container state.

```bash
sudo omurga --env production project status ./blog
omurga --json --env production project status ./blog
```

### `omurga project restart [path]` and `omurga project stop [path]`

Restart or stop project services without deleting their containers or data:

```bash
sudo omurga --env production project restart ./blog
sudo omurga --env production project stop ./blog
sudo omurga --dry-run --env production project restart ./blog
```

### `omurga project logs [path]`

Show or follow Compose logs.

Options:

- `--follow`, `-f` — keep streaming.
- `--tail <n>` — lines from the end, default `100`; use `all` for all lines.
- `--since <value>` — Docker duration or timestamp.
- `--timestamps`, `-t` — include timestamps.
- `--service <name>` — limit output; may be supplied more than once.

```bash
sudo omurga project logs ./blog
sudo omurga project logs ./blog --service app --tail 200
sudo omurga project logs ./blog --service app --service worker --follow
sudo omurga --dry-run project logs ./blog --service app
```

JSON is supported for logs only with `--dry-run`, because live log streaming is
not a single JSON document.

### `omurga project rollback [path]`

Switch to the previous healthy deployment artifacts.

```bash
sudo omurga --dry-run project rollback ./blog
sudo omurga project rollback ./blog
```

### `omurga project delete [path]`

Remove a deployed project while preserving persistent data.

Option:

- `--purge-data` — delete persistent data; must be combined with `--yes`.

```bash
sudo omurga project delete ./blog
sudo omurga --dry-run project delete ./blog --purge-data
sudo omurga --yes project delete ./blog --purge-data
```

### `omurga project list`

List deployments recorded in Omurga’s state database:

```bash
omurga project list
omurga project list --json
```

## Environment commands

The `env` group manages non-secret values in `environments/<name>.yaml`.

```bash
omurga env list [path]
omurga env show <environment> [path]
omurga env set <environment> <service> <key> <value> [path]
omurga env unset <environment> <service> <key> [path]
```

Examples:

```bash
omurga env list ./blog
omurga env show production ./blog
omurga env set production app LOG_LEVEL warning ./blog
omurga env unset production app LOG_LEVEL ./blog
```

Do not put passwords or tokens in environment overlays. Use the `secret`
commands instead.

## Secret commands

Secret values are encrypted with age and stored under Omurga’s system
directories.

```bash
omurga secret set <name> [path] --file <file-or->
omurga secret rotate <name> [path] --file <file-or->
omurga secret list [path]
omurga secret remove <name> [path]
```

Examples:

```bash
printf %s 'secret-value' | sudo omurga --env production \
  secret set database-password ./blog --file -
sudo omurga --env production secret list ./blog
printf %s 'replacement' | sudo omurga --env production \
  secret rotate database-password ./blog --file -
sudo omurga --env production secret remove database-password ./blog
```

`--file -` reads standard input. Secret values are never printed by Omurga.

## Gateway commands

The gateway group controls the system Caddy service and generated route files:

```bash
sudo omurga gateway list
sudo omurga gateway status
sudo omurga gateway validate
sudo omurga gateway reload
```

`reload` validates the complete Caddyfile before reloading Caddy. Use
`--dry-run` to see the command without executing it.

## Shared service commands

```bash
omurga service catalog
sudo omurga service install <catalog-name> [options]
omurga service list
sudo omurga service status <name>
sudo omurga service remove <name> [--purge-data --yes]
```

`service install` options:

- `--name <name>` — instance name; defaults to the catalog name.
- `--image <image>` — override the catalog image or supply an image for a custom catalog name.
- `--environment-file <path>` — root-only Compose environment file.

The built-in catalog currently includes `postgres` and `redis`:

```bash
omurga service catalog
sudo omurga service install postgres --name main \
  --environment-file /etc/omurga/services/postgres.env
sudo omurga service install redis --name cache
sudo omurga service list
sudo omurga service status cache
sudo omurga service remove cache
```

Shared-service removal preserves data. Add `--purge-data` and `--yes` only when
that data is no longer needed.

## PostgreSQL commands

All PostgreSQL commands operate on a project-scoped dependency. Add
`--instance <name>` when the project has multiple PostgreSQL dependencies.

```bash
omurga postgres status [path] --instance <name>
omurga postgres databases [path] --instance <name>
omurga postgres create-db <database> [path] --instance <name>
omurga postgres create-user <user> [path] --instance <name> --password-file <file-or->
omurga postgres shell [path] --instance <name>
omurga postgres backup [path] --instance <name> --output <path>
omurga postgres restore [path] --instance <name> --file <path> [--no-safety-backup]
```

`restore` requires `--yes`. `create-user` requires `--password-file`. The
backup output defaults to a timestamped file under
`/var/backups/omurga/staging`.

## Redis commands

```bash
omurga redis status [path] --instance <name>
omurga redis stats [path] --instance <name>
omurga redis shell [path] --instance <name>
omurga redis backup [path] --instance <name> --output <path>
omurga redis flush [path] --instance <name>
```

`flush` requires `--yes`. Redis backup output defaults to a timestamped RDB
file under `/var/backups/omurga/staging`.

## Backup commands

The common options below are available on backup commands:

- `--repository <uri>` — override `backup.destination`.
- `--password-file <path>` — Restic repository password.
- `--environment-file <path>` — root-only backend credential environment file.

```bash
omurga backup create [path] [common options] [--init]
omurga backup list [path] [common options]
omurga backup show <snapshot> [path] [common options]
omurga backup check [path] [common options]
omurga backup restore <snapshot> [path] [common options] [--target <path>]
omurga backup prune [path] [common options]
omurga backup schedule [path] [common options] [--calendar <value>]
omurga backup unschedule [path]
```

`restore` and `prune` require `--yes`. `create --init` initializes an empty
repository before creating a snapshot. `schedule` accepts `HH:MM` or a systemd
calendar expression.

## Registry commands

Registry profiles store connection metadata; Docker stores the actual login
credentials.

```bash
omurga registry add <name> <address> [--username <user>]
omurga registry list
omurga registry login <name> --password-file <file-or-> [--username <user>]
omurga registry remove <name> [--logout=false]
```

Example:

```bash
omurga registry add ghcr ghcr.io --username my-user
printf %s "$CR_PAT" | omurga registry login ghcr --password-file -
omurga registry remove ghcr
```

## Alert commands

```bash
omurga alert status
sudo omurga alert test [--channel all|telegram|email] [--message <text>]
sudo omurga alert check [--channel all|telegram|email]
sudo omurga alert schedule [--schedule <HH:MM-or-calendar>]
sudo omurga alert unschedule
```

`alert check` updates the monitor state file and sends only state changes.
`alert schedule` creates a systemd timer and requires `monitor.enabled: true`.

## Monitoring commands

```bash
sudo omurga monitoring install [options]
sudo omurga monitoring status
sudo omurga monitoring remove [--purge-data --yes]
```

Install options:

- `--bind-address <ip>` — listener address; default `127.0.0.1`.
- `--prometheus-port <port>` — host port; default `9090`.
- `--grafana-port <port>` — host port; default `3000`.
- `--grafana-admin-password-file <path>` — root-only password file. If omitted, Omurga generates one under `/etc/omurga/monitoring`.

The stack includes Prometheus, Grafana, Node Exporter, and cAdvisor. It stores
its generated Compose/configuration files under `/var/lib/omurga/services` and
its persistent data under the monitoring service data directory.
