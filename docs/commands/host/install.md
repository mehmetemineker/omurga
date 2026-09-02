---
layout: default
title: host install
description: Install or repair an individual Omurga host component.
---

# `omurga host install`

## Use it when

The host is partially configured or one component needs to be installed again.

## Scenario

Install Docker and Caddy on a host that already has Omurga directories:

```bash
sudo omurga --dry-run host install docker
sudo omurga host install docker
sudo omurga host install caddy
sudo omurga host install restic
sudo omurga host install unattended-upgrades
sudo omurga host install ufw
sudo omurga host install fail2ban
sudo omurga host install all
```

Supported components are `docker`, `caddy`, `restic`,
`unattended-upgrades`, `ufw`, `fail2ban`, and `all`.

Docker installation also configures container log rotation. UFW allows SSH,
HTTP, and HTTPS by default; use `--ssh-port` when SSH listens elsewhere.
