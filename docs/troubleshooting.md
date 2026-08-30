---
layout: default
title: Troubleshooting
description: Diagnose common Omurga, Docker, Caddy, deployment, backup, and alert problems.
---

# Troubleshooting

Start with the host doctor and a dry-run. They provide the fastest signal
without making additional changes:

```bash
sudo omurga doctor
sudo omurga --dry-run host update
sudo omurga --dry-run project deploy ./blog
```

Use `--json` when collecting output for CI or support tickets.

## `sudo` is requested repeatedly

Omurga intentionally checks root privileges for each host-changing operation.
Run a sequence inside one root shell if you want to authenticate once:

```bash
sudo -s
omurga host update
omurga project deploy ./blog
omurga alert check
exit
```

Remote profiles use non-interactive `sudo -n` by default. Configure passwordless
sudo for the deployment user or create the profile with `--sudo=false` when the
remote user is already privileged.

## UFW blocks a connection

Inspect the active policy and rules:

```bash
sudo ufw status verbose
sudo omurga doctor
```

Omurga opens the configured SSH port and TCP ports 80 and 443. If SSH uses a
different port, repair the configuration with the correct value:

```bash
sudo omurga host install ufw --ssh-port 2222
```

Other application ports must be allowed explicitly, for example:

```bash
sudo ufw allow 8080/tcp
```

If an incorrect rule removed remote access, use the provider console or local
terminal to fix the rule. Keep an existing SSH session open while changing
firewall settings.

## Docker is missing or unhealthy

```bash
omurga host detect
sudo omurga --dry-run host install docker
sudo omurga host install docker
sudo systemctl status docker
docker info
docker compose version
```

If the distribution’s Docker packages conflict with Docker CE, preview and
explicitly approve replacement:

```bash
sudo omurga --dry-run host install docker --replace-conflicting-docker
sudo omurga host install docker --replace-conflicting-docker
```

## Caddy still serves its default page

First verify the generated route and the active Caddy configuration:

```bash
sudo omurga gateway list
sudo cat /etc/caddy/Caddyfile
sudo find /etc/caddy -maxdepth 2 -type f -name '*.caddy' -print
sudo caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
sudo omurga gateway reload
```

For a local route, the request’s `Host` header must match the route domain:

```bash
curl -i -H 'Host: demo.localhost' http://127.0.0.1
```

The application port is loopback-only. Test it directly only when diagnosing
the application container:

```bash
curl -i http://127.0.0.1:<published-port>
```

If the route is present in the generated file but Caddy does not use it, check
that the main Caddyfile imports the correct directory and that the `caddy`
service account can read it:

```bash
sudo -u caddy caddy validate \
  --config /etc/caddy/Caddyfile --adapter caddyfile
sudo journalctl -u caddy -n 100 --no-pager
```

## HTTPS or Let’s Encrypt does not issue a certificate

- Use a real public DNS name; `*.localhost` is for local HTTP testing.
- Point both A/AAAA records to the server.
- Allow inbound TCP ports 80 and 443.
- Set `gateway.email` and `https: true` in the selected manifest/overlay.
- Validate and reload Caddy.

```bash
sudo omurga --env production project render ./blog --kind caddy
sudo omurga gateway validate
sudo omurga gateway reload
sudo journalctl -u caddy -n 200 --no-pager
```

## A project deployment fails

Inspect the plan, validate the manifest, then read the service logs:

```bash
omurga --env production project validate ./blog
sudo omurga --dry-run --env production project deploy ./blog
sudo omurga --env production project status ./blog
sudo omurga --env production project logs ./blog --tail all
```

Common causes include:

- the image does not support the host architecture;
- a health check command is not available in the image;
- a required secret is missing;
- a host port is already in use;
- a dependency configuration is invalid;
- a Caddy route references a port not listed in `expose`.

If a deployment fails after a previous healthy revision, Omurga attempts an
automatic rollback. You can explicitly return to the previous artifacts:

```bash
sudo omurga --env production project rollback ./blog
```

## A Raspberry Pi image does not start

Confirm the architecture and inspect the image with Docker:

```bash
dpkg --print-architecture
uname -m
docker image inspect <image>
sudo omurga --env production project status ./blog
```

Use images that publish an `arm64` variant. Avoid forcing `amd64` images on an
ARM host unless emulation is deliberately configured and its performance is
acceptable.

## A secret operation fails

Secret values must come from a file or standard input and the command must run
with root privileges:

```bash
printf %s 'value' | sudo omurga --env production \
  secret set api-token ./blog --file -
sudo omurga --env production secret list ./blog
```

Check that the age identity exists and that system secret paths are restricted:

```bash
sudo ls -l /etc/omurga/keys/identity.agekey
sudo omurga doctor
```

Do not place secret values in `environment` entries or commit secret files to
Git.

## PostgreSQL or Redis commands cannot find an instance

The data commands operate on project-scoped dependencies. Inspect the resolved
manifest and select the dependency explicitly when more than one exists:

```bash
omurga --env production project show ./blog
sudo omurga --env production postgres status ./blog --instance postgres
sudo omurga --env production redis stats ./blog --instance redis
```

Shared instances are managed through `omurga service`, not project data
commands.

## A backup fails

Check the repository, password file, backend environment file, and Restic:

```bash
sudo omurga host install restic
sudo omurga --env production backup check ./blog \
  --repository s3:s3.amazonaws.com/my-bucket/blog \
  --password-file /etc/omurga/backup/blog.password
```

Credential files must be root-only. S3-compatible credentials belong in a
root-only environment file passed with `--environment-file`. Use the backup
dry-run to inspect the selected paths:

```bash
sudo omurga --dry-run --env production backup create ./blog \
  --repository s3:s3.amazonaws.com/my-bucket/blog \
  --password-file /etc/omurga/backup/blog.password
```

## Telegram returns an empty update list

The bot must receive at least one message before Telegram exposes a chat in
`getUpdates`:

1. Open the bot conversation in Telegram.
2. Send `/start` or any message.
3. Call the Bot API `getUpdates` again.
4. Copy the numeric `chat.id` into `/etc/omurga/alerts.yaml`.

Then verify delivery:

```bash
sudo omurga alert status
sudo omurga alert test --channel telegram
```

## Monitoring is not reachable

The monitoring stack binds to loopback by default. Check the containers and
use an SSH tunnel:

```bash
sudo omurga monitoring status
ssh -L 3000:127.0.0.1:3000 user@server
```

Open `http://127.0.0.1:3000`. Prometheus is available through the same tunnel
on port 9090:

```bash
ssh -L 9090:127.0.0.1:9090 user@server
```

If direct LAN access is required, update the stack with an explicit bind
address and protect the ports with the host firewall:

```bash
sudo omurga monitoring install --bind-address 192.168.0.50
```

## Collect a support report

Avoid including secret values. The following commands collect useful redacted
operational information:

```bash
omurga version
omurga host detect --json
sudo omurga doctor --json
sudo omurga gateway status --json
sudo omurga project list --json
sudo omurga monitoring status
```
