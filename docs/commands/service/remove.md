---
layout: default
title: service remove
description: Remove a shared service while preserving data by default.
---

# `omurga service remove`

```bash
sudo omurga --dry-run service remove cache
sudo omurga service remove cache
sudo omurga --yes service remove cache --purge-data
```

The purge form permanently removes the shared service data directory.
