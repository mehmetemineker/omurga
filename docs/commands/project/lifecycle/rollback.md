---
layout: default
title: project rollback
description: Return a project to its previous healthy deployment.
---

# `omurga project rollback`

## Use it when

A new deployment passed startup but the application behaves incorrectly in
production.

## Scenario

Inspect status, preview the rollback, then switch revisions:

```bash
sudo omurga project status ./demo --env production
sudo omurga --dry-run project rollback ./demo --env production
sudo omurga project rollback ./demo --env production
sudo omurga project status ./demo --env production
```

Rollback switches the previous healthy Compose and Caddy artifacts together.
