---
layout: default
title: backup restore
description: Restore a backup snapshot into a staging target.
---

# `omurga backup restore`

## Scenario

Restore a snapshot for verification without immediately replacing live data:

```bash
sudo omurga --dry-run backup restore latest ./demo
sudo omurga backup restore latest ./demo
```

Review the staging result before promoting restored data.
