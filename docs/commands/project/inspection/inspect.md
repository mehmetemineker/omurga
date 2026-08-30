---
layout: default
title: project inspect
description: Inspect safe project configuration and deployment metadata.
---

# `omurga project inspect`

## Use it when

You want one readable summary of the manifest path, deployment revision,
generated artifact paths, service images, healthcheck presence, and gateway
routes.

```bash
omurga project inspect ./demo --env production
omurga project inspect ./demo --env production --json
```

Environment values and secret contents are intentionally not printed.
