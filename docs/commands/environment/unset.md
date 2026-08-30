---
layout: default
title: env unset
description: Remove a non-secret service environment value.
---

# `omurga env unset`

## Use it when

An environment override is no longer needed and the base manifest value
should be used again.

```bash
omurga env unset production app LOG_LEVEL ./demo
omurga env show production ./demo
```
