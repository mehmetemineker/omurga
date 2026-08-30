---
layout: default
title: project exec
description: Run a command in an active project service.
---

# `omurga project exec`

## Use it when

You need a one-off diagnostic or administration command inside a running
service without opening a full shell.

```bash
sudo omurga project exec ./demo app nginx -T
sudo omurga project exec ./demo app wget -qO- http://127.0.0.1/health
sudo omurga --dry-run project exec ./demo app env
```

Omurga resolves the active Compose slot, including blue-green deployments, and
allows only declared service names.
