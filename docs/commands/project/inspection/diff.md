---
layout: default
title: project diff
description: Compare the desired manifest revision with the active deployment.
---

# `omurga project diff`

## Use it when

You changed an image or environment and want to know whether the active host
needs a deployment.

```bash
omurga project diff ./demo --env production
omurga project diff ./demo --env production --json
```

If the command reports changes, preview the deployment with
`sudo omurga --dry-run project deploy ./demo --env production`.
