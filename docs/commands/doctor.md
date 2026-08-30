---
layout: default
title: doctor
description: Check the health of the Omurga host.
---

# `omurga doctor`

## Use it when

You need one command to check Docker, Caddy, UFW, Fail2ban, disk space,
automatic updates, container health, certificates, secrets permissions, and
Omurga state.

## Scenario

Run it after host initialization or when a deployment looks wrong:

```bash
sudo omurga doctor
sudo omurga doctor --json > doctor.json
```

Exit code `0` means healthy, `1` means warnings, and `2` means critical
failures.
