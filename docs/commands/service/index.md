---
layout: default
title: Shared service commands
description: Install and operate shared Docker services.
---

# Shared service commands

Shared services run once on the host and can be used by multiple projects.

| Goal | Command |
| --- | --- |
| See available services | [service catalog](catalog/) |
| Install a service | [service install](install/) |
| List installed services | [service list](list/) |
| Check a service | [service status](status/) |
| Remove a service | [service remove](remove/) |

## Scenario: install a shared Redis

```bash
omurga service catalog
sudo omurga --dry-run service install redis --name cache
sudo omurga service install redis --name cache
sudo omurga service status cache
```

Removal preserves data by default.
