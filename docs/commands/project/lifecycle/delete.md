---
layout: default
title: project delete
description: Remove a project while preserving data by default.
---

# `omurga project delete`

## Use it when

You no longer need a project and want to remove its containers and generated
artifacts.

```bash
sudo omurga --dry-run project delete ./demo --env production
sudo omurga project delete ./demo --env production
```

Persistent data is preserved by default. Permanent deletion requires both
flags:

```bash
sudo omurga --yes project delete ./demo --env production --purge-data
```
