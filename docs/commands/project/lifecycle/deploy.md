---
layout: default
title: project deploy
description: Reconcile a project to its desired state.
---

# `omurga project deploy`

## Use it when

An image, manifest, gateway route, or environment overlay changed and the host
should match the new desired state.

## Scenario

Preview first, then deploy:

```bash
omurga project validate ./demo --env production
sudo omurga --dry-run project deploy ./demo --env production
sudo omurga project deploy ./demo --env production
```

Omurga validates Compose and Caddy, waits for container health, and preserves a
previous healthy revision for automatic rollback.
