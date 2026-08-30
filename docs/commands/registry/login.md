---
layout: default
title: registry login
description: Authenticate Docker to a configured registry.
---

# `omurga registry login`

```bash
sudo omurga registry login ghcr --password-file /root/ghcr-password
printf %s "$REGISTRY_PASSWORD" | sudo omurga registry login ghcr --password-file -
```

The password is sent through Docker's standard input and is not a command-line
argument.
