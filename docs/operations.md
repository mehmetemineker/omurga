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

## Image deployment webhooks

Omurga webhooks deploy images only after a CI pipeline has built and pushed
them. Configure a target and generate its signing secret:

```bash
sudo omurga webhook add demo-production \
  --project demo \
  --environment production \
  --service app \
  --manifest /opt/omurga/projects/demo/omurga.yaml \
  --image-prefix ghcr.io/acme/demo
```

Store the printed secret in the CI provider, then run the webhook listener on
loopback:

```bash
sudo omurga webhook serve --listen 127.0.0.1:8090
```

Install and enable the systemd service automatically:

```bash
sudo omurga --dry-run webhook install --binary /usr/local/bin/omurga
sudo omurga webhook install --binary /usr/local/bin/omurga
sudo omurga webhook status
```

Expose the loopback listener through a Caddy HTTPS site:

```caddyfile
deploy.example.com {
    reverse_proxy 127.0.0.1:8090
}
```

The CI job must send `POST /webhooks/demo-production` with these headers:

- `X-Omurga-Event: image.published`
- `X-Omurga-Delivery: <unique-delivery-id>`
- `X-Omurga-Timestamp: <unix-seconds>`
- `X-Omurga-Signature-256: sha256=<HMAC-SHA256>`

The signature covers `<timestamp>.<raw-request-body>` and uses the webhook
secret. The JSON body must identify the configured target and include an
immutable digest:

```json
{
  "project": "demo",
  "environment": "production",
  "service": "app",
  "image": "ghcr.io/acme/demo:build-42",
  "digest": "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
}
```

A GitHub Actions shell step can create the signature after the image push:

```bash
timestamp="$(date +%s)"
payload='{"project":"demo","environment":"production","service":"app","image":"ghcr.io/acme/demo:build-42","digest":"sha256:..."}'
signature="$(printf '%s.%s' "$timestamp" "$payload" | openssl dgst -sha256 -hmac "$OMURGA_WEBHOOK_SECRET" -binary | xxd -p -c 256)"
curl --fail-with-body -X POST "https://deploy.example.com/webhooks/demo-production" \
  -H 'Content-Type: application/json' \
  -H 'X-Omurga-Event: image.published' \
  -H "X-Omurga-Delivery: ${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}" \
  -H "X-Omurga-Timestamp: $timestamp" \
  -H "X-Omurga-Signature-256: sha256=$signature" \
  --data-binary "$payload"
```

The listener rejects stale timestamps, duplicate delivery IDs, invalid HMAC
signatures, unknown payload fields, images outside the configured repository,
and non-SHA256 digests. Accepted deliveries are serialized per listener and
recorded under `/var/lib/omurga/webhooks/replay.json`. The existing deployment
health checks and rollback behavior remain in effect.

## Docker log rotation

Docker installation configures `/etc/docker/daemon.json` with the `local` log
driver, a maximum log file size of `10m`, and three rotated files per
container. This limits normal container logs to approximately 30 MB per
container and preserves other valid Docker daemon settings.

Repair the configuration on an existing Docker host with:

```bash
sudo omurga --dry-run host install docker
sudo omurga host install docker
```

New settings apply to newly created containers. Recreate existing project
containers when necessary:

```bash
sudo docker compose up -d --force-recreate
```

The `local` driver keeps `docker logs` available. Applications that write log
files inside volumes or bind mounts need a separate application policy or
`logrotate` configuration.

## Automatic security updates

Install daily unattended security updates while keeping Docker and Caddy
updates under Omurga control:

```bash
sudo omurga --dry-run host install unattended-upgrades
sudo omurga host install unattended-upgrades
```

The configuration uses the distribution’s allowed APT origins, enables daily
package-list and upgrade timers, removes unused dependencies, and disables
automatic reboot. Docker, containerd, Docker plugins, and Caddy are excluded
from unattended changes so service updates can be tested and applied with
Omurga.

Check the configuration and timers:

```bash
sudo omurga doctor
sudo systemctl status apt-daily.timer apt-daily-upgrade.timer
sudo tail -n 50 /var/log/unattended-upgrades/unattended-upgrades.log
```

Omurga reports a pending reboot through `doctor`; reboot deliberately after
checking active services and deployment state.

## UFW firewall

Install or repair the host firewall with Omurga. Preview the commands before
enabling it on a remote server:

```bash
sudo omurga --dry-run host install ufw --ssh-port 22
sudo omurga host install ufw --ssh-port 22
```

UFW denies incoming traffic by default, allows outgoing traffic, and opens
only the configured SSH port plus TCP ports 80 and 443. If SSH uses a custom
port, pass that port before enabling the firewall:

```bash
sudo omurga host install ufw --ssh-port 2222
```

Application ports are not opened automatically. Add them explicitly when
needed, then verify the effective rules:

```bash
sudo ufw allow 8080/tcp
sudo ufw status verbose
sudo omurga doctor
```

Keep an active SSH session open while changing firewall rules. On a remote
host, make sure the SSH port is allowed before enabling UFW; console access may
be required to recover from an incorrect rule.

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
expiry. It also detects sustained resource spikes for the host and managed
containers using a rolling baseline. It sends only new or changed issues and
sends a recovery notification when an issue disappears.

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
  schedule: '*-*-* *:00/1:00'
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
  spike:
    enabled: true
    baselineSamples: 3
    consecutiveSamples: 2
    cooldownMinutes: 30
    cpuIncreasePercent: 30
    memoryIncreasePercent: 20
    diskIncreasePercent: 5
    containerCPUIncreasePercent: 30
    containerMemoryIncreasePercent: 20
    cpuMinimumPercent: 70
    memoryMinimumPercent: 70
    diskMinimumPercent: 80
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
sudo omurga monitoring install --bind-address 192.0.2.10
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
omurga host add production 203.0.113.10 \
  --user deploy --identity ~/.ssh/id_ed25519
omurga host list
omurga host show production
omurga --host production doctor
omurga --host production monitoring status
omurga --host production --dry-run host update
```

Remote commands use non-interactive `sudo` by default. If the remote user does
not need sudo, create the profile with `--sudo=false`. Interactive project
shells and followed logs allocate a terminal automatically.

Remove a local profile with:

```bash
omurga host remove production
```
