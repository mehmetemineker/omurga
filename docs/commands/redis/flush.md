---
layout: default
title: redis flush
description: Remove all data from a Redis instance.
---

# `omurga redis flush`

## Use it when

The Redis instance is disposable cache data and must be cleared completely.

```bash
sudo omurga --dry-run redis flush ./demo
sudo omurga --yes redis flush ./demo
```

Do not use this for a Redis instance that contains durable application data.
