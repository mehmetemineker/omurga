---
layout: default
title: Operations
description: Backups, alerts, monitoring, registries, shared services, and remote hosts with Omurga.
---

# Operations

This guide covers the parts of Omurga that keep a deployed host reliable after
the first deployment.

## Host updates and health checks

Update package indexes and upgrade packages safely:

```bash
sudo omurga --dry-run host update
sudo omurga host update
```

Use the provider’s full distribution upgrade mode only when you understand the
distribution’s package transition behavior:

```bash
sudo omurga host update --full
```

`doctor` checks the operating system, privileges, package manager, systemd,
Docker, Compose, Caddy, configuration access, containers, reboot state, disk,
inodes, secret permissions, state database, and backup timers:

```bash
sudo omurga doctor
sudo omurga host status
sudo omurga doctor --json
```

The `host status` and `host doctor` commands are aliases for the same health
check behavior in the current CLI.

## Fail2ban

Install or repair Fail2ban with Omurga:

```bash
sudo omurga --dry-run host install fail2ban
sudo omurga host install fail2ban
```

The installer writes `/etc/fail2ban/jail.d/omurga-sshd.conf`, enables the
Fail2ban systemd service, and activates a conservative SSH jail. It allows
five failed attempts within ten minutes and bans the source for one hour.
The jail uses the systemd journal backend and is intentionally limited to SSH;
application-specific jails can be added separately under `jail.d`.

Verify the service and jail:

```bash
sudo systemctl status fail2ban
sudo fail2ban-client status
sudo fail2ban-client status sshd
```

## PostgreSQL operations

For a project-scoped PostgreSQL dependency, Omurga can show status, list
databases, create a database, create a login role, open a shell, back up, and
restore:

```bash
sudo omurga --env production postgres status ./blog
sudo omurga --env production postgres databases ./blog
sudo omurga --env production postgres create-db analytics ./blog

printf %s 'role-password' | sudo omurga --env production \
  postgres create-user reporting ./blog --password-file -

sudo omurga --env production postgres shell ./blog
sudo omurga --env production postgres backup ./blog \
  --output /var/backups/blog.dump
sudo omurga --env production postgres restore ./blog \
  --file /var/backups/blog.dump
```

Select an instance when a project has more than one PostgreSQL dependency:

```bash
sudo omurga --env production postgres databases ./blog --instance analytics
```

Restore creates a pre-restore safety dump by default. Skip it only when the
existing state is already protected:

```bash
sudo omurga --env production postgres restore ./blog \
  --file /var/backups/blog.dump --no-safety-backup
```

## Redis operations

```bash
sudo omurga --env production redis status ./blog
sudo omurga --env production redis stats ./blog
sudo omurga --env production redis shell ./blog
sudo omurga --env production redis backup ./blog \
  --output /var/backups/blog-redis.rdb
```

Flushing Redis permanently deletes all keys and requires `--yes`:

```bash
sudo omurga --yes --env production redis flush ./blog
```

Use `--instance` for projects with multiple Redis dependencies.

## Restic backups

The backup manager captures the project manifest, generated Compose and Caddy
artifacts, project data, Omurga state, encrypted secret store, and selected
PostgreSQL/Redis dumps. Configure a repository in the manifest or pass one on
the command line.

Create a backup and initialize a new repository when necessary:

```bash
sudo omurga --env production backup create ./blog \
  --repository s3:s3.amazonaws.com/my-bucket/blog \
  --password-file /etc/omurga/backup/blog.password --init
```

S3-compatible backends receive credentials through a root-only environment
file:

```bash
sudo omurga --env production backup create ./blog \
  --repository s3:s3.amazonaws.com/my-bucket/blog \
  --password-file /etc/omurga/backup/blog.password \
  --environment-file /etc/omurga/backup/s3.env
```

Inspect and verify the repository:

```bash
sudo omurga --env production backup list ./blog \
  --password-file /etc/omurga/backup/blog.password
sudo omurga --env production backup show latest ./blog \
  --password-file /etc/omurga/backup/blog.password
sudo omurga --env production backup check ./blog \
  --password-file /etc/omurga/backup/blog.password
```

Restore into a staging directory. It requires `--yes` because files in the
target may be replaced:

```bash
sudo omurga --yes --env production backup restore latest ./blog \
  --password-file /etc/omurga/backup/blog.password \
  --target /var/backups/omurga/staging/blog-restore
```

Apply the default or manifest retention policy:

```bash
sudo omurga --yes --env production backup prune ./blog \
  --password-file /etc/omurga/backup/blog.password
```

Schedule and remove a systemd timer. `03:00` is a convenient daily schedule;
systemd calendar expressions are also accepted:

```bash
sudo omurga --env production backup schedule ./blog \
  --password-file /etc/omurga/backup/blog.password --calendar 03:00
sudo omurga --env production backup unschedule ./blog
```

## Telegram and email alerts

Alert configuration lives at `/etc/omurga/alerts.yaml`. Credential values are
referenced through files and should not be committed to a repository.

Example Telegram configuration:

```yaml
telegram:
  enabled: true
  tokenFile: /etc/omurga/alerts/telegram.token
  chatId: "123456789"
```

Example SMTP configuration:

```yaml
smtp:
  enabled: true
  host: smtp.example.com
  port: 587
  username: alerts@example.com
  passwordFile: /etc/omurga/alerts/smtp.password
  from: alerts@example.com
  to:
    - ops@example.com
  tls: starttls
```

Inspect the redacted configuration and send a test:

```bash
sudo omurga alert status
sudo omurga alert test --channel telegram --message 'Omurga test'
sudo omurga alert test --channel email
sudo omurga alert test --channel all
```

The host monitor checks CPU load, memory, disk and inode usage, failed systemd
units, configured services, managed container health, and Caddy certificate
expiry. It sends only new or changed issues and sends a recovery notification
when an issue disappears.

```bash
sudo omurga alert check
sudo omurga alert schedule
sudo omurga alert schedule --schedule '02:00'
sudo omurga alert unschedule
```

Thresholds and monitored services are configured under `monitor`:

```yaml
monitor:
  enabled: true
  schedule: '*/5 * * * *'
  cpuWarningPercent: 80
  cpuCriticalPercent: 95
  memoryWarningPercent: 80
  memoryCriticalPercent: 90
  diskWarningPercent: 80
  diskCriticalPercent: 90
  certificateWarningDays: 30
  services:
    - docker
    - caddy
  certificateRoots:
    - /var/lib/caddy/.local/share/caddy
```

## Prometheus and Grafana monitoring

Install the optional four-container stack:

```bash
sudo omurga monitoring install
sudo omurga monitoring status
```

It includes Prometheus, Grafana, Node Exporter, and cAdvisor. Prometheus
scrapes host and container metrics on a dedicated Docker network. Prometheus
and Grafana bind to `127.0.0.1` by default. Use an SSH tunnel for a remote
server:

```bash
ssh -L 3000:127.0.0.1:3000 user@server
```

Open `http://127.0.0.1:3000`; the randomly generated admin password is stored
in `/etc/omurga/monitoring/grafana-admin-password`.

For direct LAN access, choose an explicit address:

```bash
sudo omurga monitoring install --bind-address 192.168.0.50
```

Remove the stack while preserving its time series and Grafana data, or purge
all monitoring data explicitly:

```bash
sudo omurga monitoring remove
sudo omurga --yes monitoring remove --purge-data
```

## Shared services

The shared service catalog currently includes PostgreSQL and Redis. Shared
services run independently from application projects on a shared Docker
network:

```bash
omurga service catalog
sudo omurga service install postgres --name main \
  --environment-file /etc/omurga/services/postgres.env
sudo omurga service install redis --name cache
sudo omurga service list
sudo omurga service status cache
sudo omurga service remove cache
```

PostgreSQL requires a root-only environment file containing its Compose
variables. Removal preserves bind-mounted data; permanently remove it only
with both flags:

```bash
sudo omurga --yes service remove cache --purge-data
```

## Docker registries

Store registry connection metadata locally, then authenticate without putting a
password in shell history:

```bash
omurga registry add ghcr.io ghcr.io --username my-user
omurga registry list
printf %s "$CR_PAT" | omurga registry login ghcr.io --password-file -
omurga registry remove ghcr.io
```

Use `--username` on login to override the profile value. Registry metadata is
stored in the user configuration directory; Docker stores the actual token in
its own credential configuration.

## Remote hosts

Remote execution uses SSH and runs the Omurga binary on the target host. The
binary must already be installed there:

```bash
omurga host add pi 192.168.0.50 \
  --user mehmet --identity ~/.ssh/id_ed25519
omurga host list
omurga host show pi
omurga --host pi doctor
omurga --host pi monitoring status
omurga --host pi --dry-run host update
```

Remote commands use non-interactive `sudo` by default. If the remote user does
not need sudo, create the profile with `--sudo=false`. Interactive project
shells and followed logs allocate a terminal automatically.

Remove a local profile with:

```bash
omurga host remove pi
```
