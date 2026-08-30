---
layout: default
title: env set
description: Set a non-secret service environment value.
---

# `omurga env set`

## Use it when

You need to change a service setting for one environment without editing the
base manifest.

```bash
omurga env set production app LOG_LEVEL info ./demo
omurga env set staging app FEATURE_FLAG_ENABLED true ./demo
```

Use this only for non-secret values. The next deployment applies the change.
