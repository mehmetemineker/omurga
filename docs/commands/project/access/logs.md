---
layout: default
title: project logs
description: Read or follow Docker Compose project logs.
---

# `omurga project logs`

## Use it when

An application is returning errors, restarting, or failing its health check.

```bash
sudo omurga project logs ./demo --service app --tail 200
sudo omurga project logs ./demo --service app --since 15m --timestamps
sudo omurga project logs ./demo --service app --follow
```

Use `--dry-run` to inspect the generated Docker command without reading logs.
