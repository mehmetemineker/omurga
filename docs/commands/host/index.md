---
layout: default
title: Host commands
description: Provision and maintain a supported Debian or Ubuntu host.
---

# Host commands

Use these commands when preparing a Debian or Ubuntu server, Raspberry Pi, or
remote Linux machine for Omurga.

| Goal | Command |
| --- | --- |
| Check the detected operating system | [`host detect`](detect/) |
| Preview or apply the complete setup | [`host init`](init/) |
| Install one component | [`host install`](install/) |
| Update system packages | [`host update`](update/) |
| Run host health checks | [`host status`](status/) or [`host doctor`](doctor/) |

## New host scenario

```bash
omurga host detect
sudo omurga --dry-run host init
sudo omurga host init
sudo omurga doctor
```

`host init` is repeatable. Use the component pages when Docker, Caddy, UFW,
Fail2ban, Restic, or automatic security updates need to be installed or
repaired individually.
