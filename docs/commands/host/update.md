---
layout: default
title: host update
description: Update Debian or Ubuntu package indexes and installed packages.
---

# `omurga host update`

## Use it when

You want to apply regular operating-system package updates.

## Scenario

Preview updates during a maintenance window, then apply them:

```bash
sudo omurga --dry-run host update
sudo omurga host update
```

Use `--full` when the provider's full distribution upgrade mode is required.
