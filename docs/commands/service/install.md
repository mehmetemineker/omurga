---
layout: default
title: service install
description: Install or update a shared Docker service.
---

# `omurga service install`

```bash
sudo omurga --dry-run service install redis --name cache
sudo omurga service install redis --name cache
```

Use `--environment-file` for a protected Compose environment file or `--image`
for a custom image.
