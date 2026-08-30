---
layout: default
title: project stop
description: Stop project services without deleting containers or data.
---

# `omurga project stop`

## Use it when

You need a temporary maintenance stop and want to preserve the project
containers and persistent data.

```bash
sudo omurga --dry-run project stop ./demo --env production
sudo omurga project stop ./demo --env production
```

Use `project restart` or `project deploy` to bring the project back.
