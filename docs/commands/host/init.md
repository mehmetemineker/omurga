---
layout: default
title: host init
description: Initialize a Debian or Ubuntu host for Omurga.
---

# `omurga host init`

## Use it when

You are preparing a host for the first project. Omurga creates its directories
and installs or configures Docker, Caddy, Restic, automatic security updates,
UFW, and Fail2ban.

## Safe first run

Preview the complete change set, then apply it:

```bash
sudo omurga --dry-run host init
sudo omurga host init
sudo omurga doctor
```

## Selective setup

Skip components that are already managed by your host:

```bash
sudo omurga host init --skip-restic --skip-fail2ban
```

Use `omurga host init --help` for all skip flags. If a distribution Docker
package conflicts with Docker CE, add `--replace-conflicting-docker`.
