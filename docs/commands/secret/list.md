---
layout: default
title: secret list
description: List secret names without revealing values.
---

# `omurga secret list`

## Use it when

You need to verify that a required secret exists without exposing its value.

```bash
sudo omurga --env production secret list ./demo
sudo omurga --env production secret list ./demo --json
```

Only names are printed.
