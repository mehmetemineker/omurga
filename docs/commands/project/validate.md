---
layout: default
title: project validate
description: Validate a project manifest and environment overlay.
---

# `omurga project validate`

## Use it when

You changed YAML or an environment overlay and want to catch errors before
rendering or deploying.

```bash
omurga project validate ./demo
omurga project validate ./demo --env production
omurga project validate ./demo --env production --json
```

Validation is read-only and does not contact Docker.
