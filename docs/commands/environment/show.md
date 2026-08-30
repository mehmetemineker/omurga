---
layout: default
title: env show
description: Show a project environment overlay.
---

# `omurga env show`

## Use it when

You want to confirm the non-secret values that will be merged into a selected
environment.

```bash
omurga env show production ./demo
omurga env show production ./demo --json
```

Do not put secret values in this file; use `secret set` instead.
