---
layout: default
title: project repair
description: Reconcile missing project containers or gateway state.
---

# `omurga project repair`

## Use it when

The project state says it is deployed but a container, generated Compose file,
or Caddy route is missing or stale.

## Scenario

Inspect, preview the repair, then apply it:

```bash
sudo omurga project inspect ./demo --env production
sudo omurga --dry-run project repair ./demo --env production
sudo omurga project repair ./demo --env production
```

Repair uses the normal deployment health checks and rollback safety. It is safe
to repeat after an interrupted operation.
